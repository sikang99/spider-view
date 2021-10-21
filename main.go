//=================================================================================
//	Filaname: main.go
// 	Function: main function of video viewer program
// 	Author: Stoney Kang, sikang@teamgrit.kr
// 	Copyright: TeamGRIT, 2021
//=================================================================================
package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"

	"github.com/gorilla/websocket"

	"github.com/pion/webrtc/v3"
)

//---------------------------------------------------------------------------------
type WsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type PGConfig struct {
	VideoWidth       int       `json:"video_width,omitempty"`
	VideoHeight      int       `json:"video_height,omitempty"`
	BitRate          int       `json:"bit_rate,omitempty"`
	KeyFrameInterval int       `json:"key_frame_interval,omitempty"`
	VideoNouse       bool      `json:"video_nouse,omitempty"`
	AudioNouse       bool      `json:"audio_nouse,omitempty"`
	VideoCodec       string    `json:"video_codec,omitempty"`
	AudioCodec       string    `json:"audio_codec,omitempty"`
	VideoType        string    `json:"video_type,omitempty"`
	AudioType        string    `json:"audio_type,omitempty"`
	VideoLabel       string    `json:"video_label,omitempty"` // video device name (label)
	AudioLabel       string    `json:"audio_label,omitempty"` // audio device name (label)
	ICEServer        string    `json:"ice_server,omitempty"`
	SpiderServer     string    `json:"spider_server,omitempty"`
	ChannelID        string    `json:"channel_id,omitempty"`
	URL              string    `json:"url,omitempty"`
	ffmpeg           *exec.Cmd `json:"-"`
}

//---------------------------------------------------------------------------------
func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

//---------------------------------------------------------------------------------
func main() {
	fmt.Printf("Spider Video Viewer, v%s (c)TeamGRIT, 2021\n", Version)

	// Default program settings
	pgConfig := &PGConfig{
		VideoWidth:       1280,
		VideoHeight:      720,
		BitRate:          1_000_000, // 1Mbps
		KeyFrameInterval: 60,
		VideoNouse:       false,
		AudioNouse:       false,
		VideoCodec:       "h264",
		AudioCodec:       "opus",
		VideoType:        "camera",
		AudioType:        "microphone",
		ICEServer:        "cobot.center:3478",
		SpiderServer:     "localhost:8267",
		ChannelID:        "bq5ame6g10l3jia3h0ng", // CoJam.Shop channel
	}

	// Handle command line options
	// flag.IntVar(&pgConfig.VideoWidth, "vwidth", pgConfig.VideoWidth, "video width to use")
	// flag.IntVar(&pgConfig.VideoHeight, "vheight", pgConfig.VideoHeight, "video height to use")
	// flag.BoolVar(&pgConfig.VideoNouse, "vnouse", pgConfig.VideoNouse, "video no use")
	// flag.BoolVar(&pgConfig.AudioNouse, "anouse", pgConfig.AudioNouse, "audio no use")
	// flag.StringVar(&pgConfig.VideoType, "vtype", pgConfig.VideoType, "video device type [camera|screen]")
	// flag.StringVar(&pgConfig.AudioType, "atype", pgConfig.AudioType, "audio device type [microphone]")
	// flag.StringVar(&pgConfig.VideoLabel, "vlabel", pgConfig.VideoLabel, "label of video device to select")
	// flag.StringVar(&pgConfig.AudioLabel, "alabel", pgConfig.AudioLabel, "label of audio device to select")
	// flag.StringVar(&pgConfig.VideoCodec, "vcodec", pgConfig.VideoCodec, "video codec to use [h264|vp8|vp9]")
	// flag.StringVar(&pgConfig.AudioCodec, "acodec", pgConfig.AudioCodec, "audio codec to use [opus]")
	flag.IntVar(&pgConfig.BitRate, "brate", pgConfig.BitRate, "bit rate of video to send in bps")
	flag.IntVar(&pgConfig.KeyFrameInterval, "kint", pgConfig.KeyFrameInterval, "key frame interval")
	flag.StringVar(&pgConfig.ICEServer, "ice", pgConfig.ICEServer, "ice server address to use")
	flag.StringVar(&pgConfig.SpiderServer, "spider", pgConfig.SpiderServer, "ice server address to use")
	flag.StringVar(&pgConfig.ChannelID, "channel", pgConfig.ChannelID, "channel id of spider server to send")
	flag.StringVar(&pgConfig.URL, "url", pgConfig.URL, "url of spider server to connect")
	flag.Parse()

	if pgConfig.URL == "" {
		pgConfig.URL = fmt.Sprintf("wss://%s/live/ws/sub?channel=%s&vcodec=%s",
			pgConfig.SpiderServer, pgConfig.ChannelID, pgConfig.VideoCodec)
	}

	err := pgConfig.prepareFFmpeg()
	if err != nil {
		log.Println(err)
		return
	}

	ws, err := ConnectWebsocketByUrl(pgConfig.URL, 1024)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	// -- webrtc configuration to use
	rtcConfig := pgConfig.SetRTCConfiguratrion()
	// rc, _ := json.Marshal(rtcConfig.SDPSemantics)
	// rc, _ := json.Marshal(rtcConfig)
	// log.Println(string(rc))

	mediaEngine := webrtc.MediaEngine{}
	// log.Println(mediaEngine)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))
	pc, err := api.NewPeerConnection(rtcConfig)
	if err != nil {
		log.Println(err)
		return
	}
	// log.Println(pc)

	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Println("w.OnICEConnectionState:", connectionState)
		if connectionState == webrtc.ICEConnectionStateDisconnected {
			os.Exit(0)
		}
	})

	pc.OnICECandidate(func(iceCandidate *webrtc.ICECandidate) {
		log.Println("w.OnICECandidate:", iceCandidate)
		// send candidates to the peer (server)
		if iceCandidate != nil {
			candidate := iceCandidate.ToJSON()
			data, err := json.Marshal(candidate)
			if err != nil {
				log.Println("json.Marshal", err)
				return
			}
			err = ws.WriteJSON(&WsMessage{
				Type: "candidate2",
				Data: string(data),
			})
			if err != nil {
				log.Println("json.Marshal", err)
				return
			}
		}
	})

	err = SendOfferByWebsocket(ws, pc)
	if err != nil {
		log.Println(err)
		return
	}

	err = ProcMessageByWebsocket(ws, pc)
	if err != nil {
		log.Println(err)
		return
	}
}

//---------------------------------------------------------------------------------
func (d *PGConfig) prepareFFmpeg() (err error) {
	d.ffmpeg = exec.Command("ffmpeg", "-i", "pipe:0", "-pix_fmt", "bgr24", "-s",
		strconv.Itoa(d.VideoWidth)+"x"+strconv.Itoa(d.VideoHeight), "-f", "rawvideo", "pipe:1") //nolint
	if d.ffmpeg == nil {
		err = fmt.Errorf("ffmpeg exec error")
		log.Println(err)
		return
	}

	ffmpegErr, err := d.ffmpeg.StderrPipe()
	if err != nil {
		log.Println(err)
		return
	}

	err = d.ffmpeg.Start()
	if err != nil {
		log.Println(err)
		return
	}

	go func() {
		scanner := bufio.NewScanner(ffmpegErr)
		for scanner.Scan() {
			log.Println(scanner.Text())
		}
	}()
	return
}

//---------------------------------------------------------------------------------
func (d *PGConfig) SetRTCConfiguratrion() (rtcConfig webrtc.Configuration) {
	log.Println("i.SetRTCConfiguratrion:", d.ICEServer)

	rtcConfig = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				//URLs: []string{"stun:stun.l.google.com:19302"},
				URLs: []string{"stun:" + d.ICEServer},
			},
			{
				URLs: []string{
					"turn:" + d.ICEServer + "?transport=udp",
					"turn:" + d.ICEServer + "?transport=tcp",
				},
				Username:   "teamgrit",
				Credential: "teamgrit8266",
			},
		},
		ICETransportPolicy: webrtc.ICETransportPolicyAll, // Policy[Relay|All]
		PeerIdentity:       "spider-device",
		SDPSemantics:       webrtc.SDPSemanticsUnifiedPlan,
	}
	return
}

//---------------------------------------------------------------------------------
func ConnectWebsocketByUrl(url string, bsize int) (ws *websocket.Conn, err error) {
	log.Println("i.ConnectUrlByWebsocket:", url)

	var dialer = websocket.Dialer{
		ReadBufferSize:  bsize,
		WriteBufferSize: bsize,
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	ws, _, err = dialer.Dial(url, nil)
	if err != nil {
		log.Println(err)
		return
	}
	return
}

//---------------------------------------------------------------------------------
func SendOfferByWebsocket(ws *websocket.Conn, pc *webrtc.PeerConnection) (err error) {
	defer log.Println("o.SendOfferByWebsocket", err)

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Println(err)
		return
	}
	// log.Println(offer)

	err = pc.SetLocalDescription(offer)
	if err != nil {
		log.Println(err)
		return
	}

	err = ws.WriteJSON(&WsMessage{
		Type: "offer",
		Data: offer.SDP,
	})
	if err != nil {
		log.Println(err)
		return
	}

	return
}

//---------------------------------------------------------------------------------
func ProcMessageByWebsocket(ws *websocket.Conn, pc *webrtc.PeerConnection) (err error) {
	defer log.Println("o.ProcMessageByWebsocket", err)

	for {
		m := WsMessage{}
		err = ws.ReadJSON(&m)
		if err != nil {
			log.Println(err)
			return
		}

		switch m.Type {
		case "ping", "joins":
			log.Println("[msg]", m)
		case "offer":
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  m.Data,
			}
			err = pc.SetRemoteDescription(offer)
			if err != nil {
				log.Println("pc.SetRemoteDescription", err)
				return
			}
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				log.Println(err)
				return err
			}
			err = pc.SetLocalDescription(answer)
			if err != nil {
				log.Println("pc.SetLocalDescription", err)
				return err
			}
			ws.WriteJSON(&WsMessage{
				Type: "answer",
				Data: answer.SDP,
			})
		case "answer":
			answer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  m.Data,
			}
			err = pc.SetRemoteDescription(answer)
			if err != nil {
				log.Println("pc.SetRemoteDescription", err)
				return
			}
		case "candidate": // old style, pion/mediadevices
			// log.Println("msg.data", m.Data)
			candidate := webrtc.ICECandidateInit{
				Candidate: m.Data,
			}
			err = pc.AddICECandidate(candidate)
			if err != nil {
				log.Println("pc.AddICECandidate:", err)
				return
			}
		case "candidate2": // new style, standard type
			// log.Println("msg.data", m.Data)
			candidate := webrtc.ICECandidateInit{}
			err = json.Unmarshal([]byte(m.Data), &candidate)
			if err != nil {
				log.Println("json.Unmarshal:", err)
				break
			}
			err = pc.AddICECandidate(candidate)
			if err != nil {
				log.Println("pc.AddICECandidate:", err)
				return
			}
		default:
			log.Println("unknown [msg]", m)
		}
	}
}

//=================================================================================
