//=================================================================================
//	Filaname: main.go
// 	Function: main function of device shooter program
// 	Author: Stoney Kang, sikang@teamgrit.kr
// 	Copyright: TeamGRIT, 2021
//=================================================================================
package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/pion/mediadevices"
	// "github.com/pion/mediadevices/pkg/codec/mmal"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	"github.com/pion/mediadevices/pkg/codec/x264"
	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"

	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	_ "github.com/pion/mediadevices/pkg/driver/screen"
	// _ "github.com/pion/mediadevices/pkg/driver/audiotest"
	// _ "github.com/pion/mediadevices/pkg/driver/videotest"
)

//---------------------------------------------------------------------------------
type WsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type PGConfig struct {
	VideoWidth       int                          `json:"video_width,omitempty"`
	VideoHeight      int                          `json:"video_height,omitempty"`
	BitRate          int                          `json:"bit_rate,omitempty"`
	KeyFrameInterval int                          `json:"key_frame_interval,omitempty"`
	VideoNouse       bool                         `json:"video_nouse,omitempty"`
	AudioNouse       bool                         `json:"audio_nouse,omitempty"`
	VideoCodec       string                       `json:"video_codec,omitempty"`
	AudioCodec       string                       `json:"audio_codec,omitempty"`
	VideoType        string                       `json:"video_type,omitempty"`
	AudioType        string                       `json:"audio_type,omitempty"`
	VideoLabel       string                       `json:"video_label,omitempty"` // video device name (label)
	AudioLabel       string                       `json:"audio_label,omitempty"` // audio device name (label)
	ICEServer        string                       `json:"ice_server,omitempty"`
	SpiderServer     string                       `json:"spider_server,omitempty"`
	ChannelID        string                       `json:"channel_id,omitempty"`
	URL              string                       `json:"url,omitempty"`
	VideoDevice      mediadevices.MediaDeviceInfo `json:"video_device,omitempty"`
	AudioDevice      mediadevices.MediaDeviceInfo `json:"audio_device,omitempty"`
}

//---------------------------------------------------------------------------------
func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

//---------------------------------------------------------------------------------
func main() {
	fmt.Printf("Spider Device Shooter, v%s (c)TeamGRIT, 2021\n", Version)

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
	flag.IntVar(&pgConfig.VideoWidth, "vwidth", pgConfig.VideoWidth, "video width to use")
	flag.IntVar(&pgConfig.VideoHeight, "vheight", pgConfig.VideoHeight, "video height to use")
	flag.BoolVar(&pgConfig.VideoNouse, "vnouse", pgConfig.VideoNouse, "video no use")
	flag.BoolVar(&pgConfig.AudioNouse, "anouse", pgConfig.AudioNouse, "audio no use")
	flag.StringVar(&pgConfig.VideoType, "vtype", pgConfig.VideoType, "video device type [camera|screen]")
	flag.StringVar(&pgConfig.AudioType, "atype", pgConfig.AudioType, "audio device type [microphone]")
	flag.StringVar(&pgConfig.VideoLabel, "vlabel", pgConfig.VideoLabel, "label of video device to select")
	flag.StringVar(&pgConfig.AudioLabel, "alabel", pgConfig.AudioLabel, "label of audio device to select")
	flag.StringVar(&pgConfig.VideoCodec, "vcodec", pgConfig.VideoCodec, "video codec to use [h264|vp8|vp9]")
	flag.StringVar(&pgConfig.AudioCodec, "acodec", pgConfig.AudioCodec, "audio codec to use [opus]")
	flag.IntVar(&pgConfig.BitRate, "brate", pgConfig.BitRate, "bit rate of video to send in bps")
	flag.IntVar(&pgConfig.KeyFrameInterval, "kint", pgConfig.KeyFrameInterval, "key frame interval")
	flag.StringVar(&pgConfig.ICEServer, "ice", pgConfig.ICEServer, "ice server address to use")
	flag.StringVar(&pgConfig.SpiderServer, "spider", pgConfig.SpiderServer, "ice server address to use")
	flag.StringVar(&pgConfig.ChannelID, "channel", pgConfig.ChannelID, "channel id of spider server to send")
	flag.StringVar(&pgConfig.URL, "url", pgConfig.URL, "url of spider server to connect")
	flag.Parse()

	if pgConfig.URL == "" {
		pgConfig.URL = fmt.Sprintf("wss://%s/live/ws/pub?channel=%s&vcodec=%s",
			pgConfig.SpiderServer, pgConfig.ChannelID, pgConfig.VideoCodec)
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

	ListDevices()
	pgConfig.VideoDevice, err = GetDeviceInfoByType(
		mediadevices.MediaDeviceType(1),       // video(1)
		driver.DeviceType(pgConfig.VideoType), // camera, screen
		pgConfig.VideoLabel)                   // number or name of device
	if err != nil {
		log.Println(err)
		return
	}

	pgConfig.AudioDevice, err = GetDeviceInfoByType(
		mediadevices.MediaDeviceType(2),       // audio(2)
		driver.DeviceType(pgConfig.AudioType), // micrphohone, (speaker?)
		pgConfig.AudioLabel)                   // number or name of device
	if err != nil {
		log.Println(err)
		return
	}

	codecSelector, err := pgConfig.SetMediaCodecSelector()
	if err != nil {
		log.Println(err)
		return
	}

	mediaEngine := webrtc.MediaEngine{}
	codecSelector.Populate(&mediaEngine)
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

	ms, err := pgConfig.GetMediaStreamByNouse(codecSelector)
	if err != nil {
		log.Println(err)
		return
	}
	// log.Println(ms)

	for _, track := range ms.GetTracks() {
		log.Println("Track:", track.Kind(), track.ID(), track.StreamID())

		track.OnEnded(func(err error) {
			log.Println("w.OnEnded", track.ID(), err)
		})

		_, err = pc.AddTransceiverFromTrack(track,
			webrtc.RtpTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionSendonly,
			},
		)
		if err != nil {
			log.Println(err)
			return
		}
	}

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
func (d *PGConfig) GetMediaStreamByNouse(cs *mediadevices.CodecSelector) (ms mediadevices.MediaStream, err error) {
	log.Println("i.GetMediaStreamByNouse:", d.AudioNouse, d.VideoNouse)

	var video, audio mediadevices.MediaOption
	switch d.VideoType {
	case "camera":
		if !d.VideoNouse {
			video = func(c *mediadevices.MediaTrackConstraints) {
				c.DeviceID = prop.String(d.VideoDevice.DeviceID)
				c.FrameFormat = prop.FrameFormat(frame.FormatI420)
				c.Width = prop.Int(d.VideoWidth)
				c.Height = prop.Int(d.VideoHeight)
			}
		}
		if !d.AudioNouse {
			audio = func(c *mediadevices.MediaTrackConstraints) {
				c.DeviceID = prop.String(d.AudioDevice.DeviceID)
			}
		}
		ms, err = mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
			Video: video,
			Audio: audio,
			Codec: cs,
		})
	case "screen":
		if !d.VideoNouse {
			video = func(c *mediadevices.MediaTrackConstraints) {
				c.DeviceID = prop.String(d.VideoDevice.DeviceID)
			}
		}
		if !d.AudioNouse {
			audio = func(c *mediadevices.MediaTrackConstraints) {
				c.DeviceID = prop.String(d.AudioDevice.DeviceID)
			}
		}
		ms, err = mediadevices.GetDisplayMedia(mediadevices.MediaStreamConstraints{
			Video: video,
			Audio: audio,
			Codec: cs,
		})
	default:
		err = fmt.Errorf("unknown device type: %s, %s", d.VideoType, d.AudioType)
	}
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
func (d *PGConfig) SetMediaCodecSelector() (cs *mediadevices.CodecSelector, err error) {
	log.Println("i.SetMediaCodecSelector:", d.VideoCodec, d.AudioCodec)

	opusParams, err := opus.NewParams()
	if err != nil {
		log.Println(err)
		return nil, err
	}

	switch d.VideoCodec {
	case "h264", "x264":
		x264Params, err := x264.NewParams()
		if err != nil {
			log.Println(err)
			return nil, err
		}
		x264Params.Preset = x264.PresetSuperfast // Superfast: High, Ultrafast: Constrained Baseline
		x264Params.BitRate = d.BitRate
		x264Params.KeyFrameInterval = d.KeyFrameInterval

		cs = mediadevices.NewCodecSelector(
			mediadevices.WithVideoEncoders(&x264Params),
			mediadevices.WithAudioEncoders(&opusParams),
		)
	// case "mmal": // for Raspberry Pi
	// 	mmalParams, err := mmal.NewParams()
	// 	if err != nil {
	// 		log.Println(err)
	// 		return cs, err
	// 	}

	// 	cs = mediadevices.NewCodecSelector(
	// 		mediadevices.WithVideoEncoders(&mmalParams),
	// 		mediadevices.WithAudioEncoders(&opusParams),
	// 	)
	// case "mmapi": // for Jetson Multimedia API
	case "vp8":
		vp8Params, err := vpx.NewVP8Params()
		if err != nil {
			log.Println(err)
			return cs, err
		}

		cs = mediadevices.NewCodecSelector(
			mediadevices.WithVideoEncoders(&vp8Params),
			mediadevices.WithAudioEncoders(&opusParams),
		)
	case "vp9":
		vp9Params, err := vpx.NewVP9Params()
		if err != nil {
			log.Println(err)
			return cs, err
		}

		cs = mediadevices.NewCodecSelector(
			mediadevices.WithVideoEncoders(&vp9Params),
			mediadevices.WithAudioEncoders(&opusParams),
		)
	default:
		err = fmt.Errorf("not supported codecs: %s, %s", d.VideoCodec, d.AudioCodec)
	}

	return
}

//---------------------------------------------------------------------------------
func CaseInsensitiveContains(s, substr string) bool {
	s, substr = strings.ToUpper(s), strings.ToUpper(substr)
	return strings.Contains(s, substr)
}

func ListDevices() {
	mds := mediadevices.EnumerateDevices()
	for _, md := range mds {
		log.Println("[device]", md.DeviceID, md.Kind, md.Label, md.DeviceType)
	}
}

func GetDeviceInfoByType(kind mediadevices.MediaDeviceType, dtype driver.DeviceType, label string) (smd mediadevices.MediaDeviceInfo, err error) {
	log.Println("i.GetDeviceInfoByType:", kind, dtype, label)

	mds := mediadevices.EnumerateDevices()
	for _, md := range mds {
		if md.Kind == kind {
			if md.DeviceType == dtype {
				if label == "" && smd.DeviceID == "" {
					smd = md
				} else {
					if CaseInsensitiveContains(md.Label, label) && smd.DeviceID == "" {
						smd = md
					}
				}
			}
		}
	}
	if smd.DeviceID == "" {
		err = fmt.Errorf("no device selected for %v, %v, %s", kind, dtype, label)
		log.Println(err)
	} else {
		log.Println("select>", smd.DeviceID, smd.Kind, smd.Label, smd.DeviceType)
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
