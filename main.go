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
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"gocv.io/x/gocv"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media/ivfwriter"
)

//---------------------------------------------------------------------------------
type WsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type Program struct {
	Ok               bool   `json:"ok"`
	VideoWidth       int    `json:"video_width,omitempty"`
	VideoHeight      int    `json:"video_height,omitempty"`
	BitRate          int    `json:"bit_rate,omitempty"`
	KeyFrameInterval int    `json:"key_frame_interval,omitempty"`
	VideoNouse       bool   `json:"video_nouse,omitempty"`
	AudioNouse       bool   `json:"audio_nouse,omitempty"`
	VideoCodec       string `json:"video_codec,omitempty"`
	AudioCodec       string `json:"audio_codec,omitempty"`
	VideoType        string `json:"video_type,omitempty"`
	AudioType        string `json:"audio_type,omitempty"`
	VideoLabel       string `json:"video_label,omitempty"` // video device name (label)
	AudioLabel       string `json:"audio_label,omitempty"` // audio device name (label)
	ICEServer        string `json:"ice_server,omitempty"`
	SpiderServer     string `json:"spider_server,omitempty"`
	ChannelID        string `json:"channel_id,omitempty"`
	URL              string `json:"url,omitempty"`
	// -- Internal handling parts
	pc     *webrtc.PeerConnection
	ws     *websocket.Conn
	msgch  chan WsMessage
	ffmpeg struct {
		cmd    *exec.Cmd
		stdin  io.WriteCloser
		stdout io.ReadCloser
		stderr io.ReadCloser
	} `json:"-"`
}

//---------------------------------------------------------------------------------
func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

//---------------------------------------------------------------------------------
func main() {
	fmt.Printf("Spider Video Viewer, v%s (c)TeamGRIT, 2021\n", Version)

	// Default program settings
	pg := &Program{
		Ok:               true,
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
		msgch:            make(chan WsMessage, 2),
	}

	// Handle command line options
	// flag.IntVar(&pg.VideoWidth, "vwidth", pg.VideoWidth, "video width to use")
	// flag.IntVar(&pg.VideoHeight, "vheight", pg.VideoHeight, "video height to use")
	// flag.BoolVar(&pg.VideoNouse, "vnouse", pg.VideoNouse, "video no use")
	// flag.BoolVar(&pg.AudioNouse, "anouse", pg.AudioNouse, "audio no use")
	// flag.StringVar(&pg.VideoType, "vtype", pg.VideoType, "video device type [camera|screen]")
	// flag.StringVar(&pg.AudioType, "atype", pg.AudioType, "audio device type [microphone]")
	// flag.StringVar(&pg.VideoLabel, "vlabel", pg.VideoLabel, "label of video device to select")
	// flag.StringVar(&pg.AudioLabel, "alabel", pg.AudioLabel, "label of audio device to select")
	// flag.StringVar(&pg.VideoCodec, "vcodec", pg.VideoCodec, "video codec to use [h264|vp8|vp9]")
	// flag.StringVar(&pg.AudioCodec, "acodec", pg.AudioCodec, "audio codec to use [opus]")
	flag.IntVar(&pg.BitRate, "brate", pg.BitRate, "bit rate of video to send in bps")
	flag.IntVar(&pg.KeyFrameInterval, "kint", pg.KeyFrameInterval, "key frame interval")
	flag.StringVar(&pg.ICEServer, "ice", pg.ICEServer, "ice server address to use")
	flag.StringVar(&pg.SpiderServer, "spider", pg.SpiderServer, "ice server address to use")
	flag.StringVar(&pg.ChannelID, "channel", pg.ChannelID, "channel id of spider server to send")
	flag.StringVar(&pg.URL, "url", pg.URL, "url of spider server to connect")
	flag.Parse()

	if pg.URL == "" {
		pg.URL = fmt.Sprintf("wss://%s/live/ws/sub?channel=%s&vcodec=%s",
			pg.SpiderServer, pg.ChannelID, pg.VideoCodec)
	}

	err := pg.openFFmpeg()
	if err != nil {
		log.Println(err)
		return
	}
	defer pg.closeFFmpeg()

	ws, err := pg.connectWebsocketByUrl(pg.URL, 1024)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	// -- webrtc configuration to use
	rtcConfig := pg.setRTCConfiguratrion()
	// rc, _ := json.Marshal(rtcConfig.SDPSemantics)
	// rc, _ := json.Marshal(rtcConfig)
	// log.Println(string(rc))

	me := webrtc.MediaEngine{}
	me.RegisterDefaultCodecs()

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me))
	pc, err := api.NewPeerConnection(rtcConfig)

	if err != nil {
		log.Println(err)
		return
	}
	// log.Println(pc)

	ivfWriter, err := ivfwriter.NewWith(pg.ffmpeg.stdin)
	if err != nil {
		log.Println(err)
		return
	}

	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Println("w.OnICEConnectionState:", connectionState)
		if connectionState == webrtc.ICEConnectionStateDisconnected {
			os.Exit(0)
		}
	})

	pc.OnICECandidate(func(iceCandidate *webrtc.ICECandidate) {
		log.Println("w.OnICECandidate:", iceCandidate)
		// send candidates to the peer (server)
		// if iceCandidate != nil {
		// 	candidate := iceCandidate.ToJSON()
		// 	data, err := json.Marshal(candidate)
		// 	if err != nil {
		// 		log.Println("json.Marshal", err)
		// 		return
		// 	}
		// 	pg.msgch <- WsMessage{
		// 		Type: "send-candidate2",
		// 		Data: string(data),
		// 	}
		// }
	})

	pc.OnTrack(func(track *webrtc.Track, receiver *webrtc.RTPReceiver) {
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				err := pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
				if err != nil {
					log.Println(err)
					return
				}
			}
		}()

		log.Printf("Track has started, of type %d: %s \n",
			track.PayloadType(), track.Codec().RTPCodecCapability.MimeType)
		for {
			// Read RTP packets being sent to Pion
			rtp, err := track.ReadRTP()
			if err != nil {
				log.Println(err)
				return
			}

			err = ivfWriter.WriteRTP(rtp)
			if err != nil {
				log.Println(err)
				return
			}
		}
	})

	// go pg.detectMotion()

	err = pg.sendOfferByWebsocket(ws, pc)
	if err != nil {
		log.Println(err)
		return
	}

	err = pg.procMessageByWebsocket(ws, pc)
	if err != nil {
		log.Println(err)
		return
	}

}

//---------------------------------------------------------------------------------
func (d *Program) openFFmpeg() (err error) {
	log.Println("i.openFFmpeg:", "bgr24")

	d.ffmpeg.cmd = exec.Command("ffmpeg", "-i", "pipe:0", "-pix_fmt", "bgr24", "-s",
		strconv.Itoa(d.VideoWidth)+"x"+strconv.Itoa(d.VideoHeight), "-f", "rawvideo", "pipe:1") //nolint
	if d.ffmpeg.cmd == nil {
		err = fmt.Errorf("ffmpeg exec error")
		log.Println(err)
		return
	}

	d.ffmpeg.stdin, _ = d.ffmpeg.cmd.StdinPipe()
	d.ffmpeg.stdout, _ = d.ffmpeg.cmd.StdoutPipe()
	d.ffmpeg.stderr, err = d.ffmpeg.cmd.StderrPipe()
	if err != nil {
		log.Println(err)
		return
	}

	err = d.ffmpeg.cmd.Start()
	if err != nil {
		log.Println(err)
		return
	}

	go func() {
		scanner := bufio.NewScanner(d.ffmpeg.stderr)
		for scanner.Scan() {
			log.Println(scanner.Text())
		}
	}()
	return
}

func (d *Program) closeFFmpeg() (err error) {
	d.ffmpeg.cmd = nil
	return
}

//---------------------------------------------------------------------------------
func (d *Program) detectMotion() (err error) {
	log.Println("i.detectMotion")

	window := gocv.NewWindow("Spider Video Viewer")
	defer window.Close() //nolint

	img := gocv.NewMat()
	defer img.Close() //nolint

	for d.Ok {
		buf := make([]byte, d.VideoWidth*d.VideoHeight*3)
		_, err := io.ReadFull(d.ffmpeg.stdout, buf)
		if err != nil {
			log.Println(err)
			continue
		}
		img, _ := gocv.NewMatFromBytes(d.VideoHeight, d.VideoWidth, gocv.MatTypeCV8UC3, buf)
		if img.Empty() {
			continue
		}

		window.IMShow(img)
		if window.WaitKey(1) == 27 {
			break
		}
	}

	return
}

//---------------------------------------------------------------------------------
func (d *Program) setRTCConfiguratrion() (rtcConfig webrtc.Configuration) {
	log.Println("i.setRTCConfiguratrion:", d.ICEServer)

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
func (d *Program) connectWebsocketByUrl(url string, bsize int) (ws *websocket.Conn, err error) {
	log.Println("i.connectUrlByWebsocket:", url)

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

	d.ws = ws
	return
}

//---------------------------------------------------------------------------------
func (d *Program) sendOfferByWebsocket(ws *websocket.Conn, pc *webrtc.PeerConnection) (err error) {
	log.Println("i.sendOfferByWebsocket")
	defer log.Println("o.sendOfferByWebsocket", err)

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

	d.msgch <- WsMessage{
		Type: "send-offer",
		Data: offer.SDP,
	}
	return
}

//---------------------------------------------------------------------------------
func (d *Program) procMessageByWebsocket(ws *websocket.Conn, pc *webrtc.PeerConnection) (err error) {
	log.Println("i.procMessageByWebsocket")
	defer log.Println("o.ProcMessageByWebsocket", err)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for d.Ok {
		select {
		case <-ticker.C:
			ws.WriteJSON(&WsMessage{
				Type: "ping",
			})
		case m, ok := <-d.msgch:
			if !ok {
				err = fmt.Errorf("msgch recv error")
				log.Println(err)
				return
			}
			// log.Println(m.Type)
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
			case "send-offer":
				ws.WriteJSON(&WsMessage{
					Type: "offer",
					Data: m.Data,
				})
			case "send-candidate2":
				ws.WriteJSON(&WsMessage{
					Type: "candidate2",
					Data: m.Data,
				})
			default:
				log.Println("unknown [msg]", m)
			}
		}
	}
	return
}

//=================================================================================
