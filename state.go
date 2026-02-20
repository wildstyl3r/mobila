package main

import (
	"context"
	"fmt"
	"image"
	"io"
	"mobila/pb"
	"slices"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-msgio/pbio"
	"github.com/pion/mediadevices/pkg/io/audio"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/wave"
)

const myProtocolID protocol.ID = "/mobila/1.0.0"

var blackImg image.Image = image.NewRGBA(image.Rect(0, 0, 1, 1))

type Libp2pStreamReader struct {
	mu     sync.Mutex
	stream pbio.Reader
}

type Libp2pStreamWriter struct {
	mu     sync.Mutex
	stream pbio.Writer
}

type State struct {
	Node     *Libp2pNode
	Store    *Store
	OwnID    string
	Contacts map[string]Contact
	Chats    []Chat

	SelectedChat      *Chat
	Videos            map[string]*VideoWidget
	ChatPeersShuffled []string

	VideoOn           bool
	VideoMutex        sync.RWMutex
	AudioOn           bool
	AudioMutex        sync.RWMutex
	InitChunk         []byte
	SequenceNumber    uint64
	StreamActive      bool
	PeerStreamWriters map[string]*Libp2pStreamWriter
	IncomingStreams   map[string]chan struct{}
	OutgoingStreams   map[string]struct{}

	mu sync.RWMutex
}

func NewState() *State {
	return &State{
		Contacts: make(map[string]Contact),
	}
}
func (s *State) Shutdown() {
	s.mu.Lock()
	{
		if s.Node != nil {
			s.Node.Shutdown()
		}
		if s.Store != nil {
			s.Store.Close()
		}
	}
	s.mu.Unlock()
}

func (s *State) ReloadContactsAndChats() (err error) {
	s.mu.Lock()
	{
		s.Contacts, err = s.Store.GetAllContacts()
		if err != nil {
			fmt.Printf("error loading contacts: %v\n", err)
		}
		s.Chats, err = s.Store.GetChatList()
		if err != nil {
			fmt.Printf("error loading chats: %v\n", err)
		}
	}
	s.mu.Unlock()
	findList := []string{}
	s.mu.RLock()
	{
		for peerID := range s.Contacts {
			if _, exist := s.PeerStreamWriters[peerID]; !exist {
				findList = append(findList, peerID)
			}
		}
	}
	s.mu.RUnlock()
	for _, peerIDstr := range findList {
		go func() {
			peerID, err := peer.Decode(peerIDstr)
			if err != nil {
				fmt.Printf("error trying to decode peer.ID from string %s\n", peerID)
				return
			}
			for {
				if s.Node.DHT.RoutingTable().Size() > 0 {
					break
				}
				time.Sleep(1 * time.Second) // Короткая пауза, чтобы не нагружать CPU
			}
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			peerInfo, err := s.Node.DHT.FindPeer(ctx, peerID)
			if err != nil {
				if err == context.DeadlineExceeded {
					fmt.Println("Поиск прекращен: превышено время ожидания (1 минута).")
				} else {
					fmt.Printf("Ошибка при поиске пира: %v\n", err)
				}
				return
			}
			fmt.Printf("Пир найден! Адреса: %v\n", peerInfo.Addrs)
		}()
	}
	return err
}

func (s *State) Init() (err error) {
	s.ReloadContactsAndChats()
	s.mu.Lock()
	{
		s.VideoOn = true
		s.Node.Host.SetStreamHandler(myProtocolID,
			func(stream network.Stream) {
				peerID := stream.Conn().RemotePeer().String()
				s.mu.Lock()
				{
					s.PeerStreamWriters[peerID] = &Libp2pStreamWriter{stream: pbio.NewDelimitedWriter(stream)}
				}
				s.mu.Unlock()
				reader := pbio.NewDelimitedReader(stream, 20*1024)
				defer stream.Close()
				for {
					var pbMsg pb.DataPacket
					if err := reader.ReadMsg(&pbMsg); err != nil {
						s.mu.Lock()
						{
							stream.Reset()
							delete(s.PeerStreamWriters, peerID)
						}
						s.mu.Unlock()
						return
					}
					switch datapacket := pbMsg.Msg.(type) {
					case *pb.DataPacket_Ping:
						s.mu.RLock()
						{
							if safestream, ok := s.PeerStreamWriters[peerID]; ok {
								safestream.mu.Lock()
								safestream.stream.WriteMsg(&pb.DataPacket{Msg: &pb.DataPacket_Pong{Pong: &pb.Pong{}}})
								safestream.mu.Unlock()
							}
						}
						s.mu.RUnlock()
					case *pb.DataPacket_Pong:
						// do nothing
					case *pb.DataPacket_ResendStatic:
						rs := datapacket.ResendStatic
						s.mu.RLock()
						{
							for c := range s.Chats {
								if s.Chats[c].ID == rs.ChatId && slices.Contains(s.Chats[c].Peers, peerID) {
									msg := s.Chats[c].GetMessage(rs.MessageId)
									if msg != nil {
										if safestream, ok := s.PeerStreamWriters[peerID]; ok {
											safestream.mu.Lock()
											safestream.stream.WriteMsg(&pb.DataPacket{Msg: &pb.DataPacket_Static{
												Static: &pb.Static{
													ChatId:        rs.ChatId,
													MessageId:     msg.ID,
													PrevMessageId: msg.Prev,
													Data:          []byte(msg.Text),
													AuthorId:      msg.Author,
													MimeType:      "text/plain",
												},
											}})
											safestream.mu.Unlock()
										}
									}
									break
								}
							}
						}
						s.mu.RUnlock()
					case *pb.DataPacket_Static:
					case *pb.DataPacket_StreamChunk:
					case *pb.DataPacket_StreamInfo:
					case *pb.DataPacket_StreamInfoResponse:
					default:
						panic(fmt.Sprintf("unexpected pb.isDataPacket_Msg: %#v", datapacket))
					}
				}

			})
	}
	s.mu.Unlock()

	go func() {
		for {
			s.mu.RLock()
			{
				if s.StreamActive && s.SelectedChat != nil {
					for _, peerID := range s.SelectedChat.Peers {
						if peerID != s.Node.Host.ID().String() {
							if safeStream, ok := s.PeerStreamWriters[peerID]; ok {
								safeStream.mu.Lock()
								{
									err := safeStream.stream.WriteMsg(&pb.DataPacket{
										Msg: &pb.DataPacket_StreamInfo{
											StreamInfo: &pb.StreamInfo{
												Status: pb.StreamInfo_ACTIVE,
												ChatId: s.SelectedChat.ID,
											},
										},
									})
									if err != nil {
										fmt.Printf("error marshalling or sending STREAM INFO : ACTIVE [%v]\n", err)
									}
								}
								safeStream.mu.Unlock()
							}
						}
					}
				}
			}
			s.mu.RUnlock()
			time.Sleep(5 * time.Second)
		}
	}()
	return
}

func (s *State) AddContact(c Contact) {
	exist := false
	s.mu.RLock()
	{
		if _, exist = s.Contacts[c.ID]; exist {
			fmt.Printf("contact already exists: %s\n", c.Alias)
		}
	}
	s.mu.RUnlock()
	if exist {
		return
	}

	fmt.Printf("add contact: %s\n", c.Alias)

	s.mu.Lock()
	{
		if s.Contacts == nil {
			s.Contacts = make(map[string]Contact)
		}
		s.Contacts[c.ID] = c
		s.Store.AddContact(c)
		s.Store.CreateNewChat(Chat{ID: uuid.NewString(), Name: c.Alias, Peers: []string{c.ID, s.Node.Host.ID().String()}})
	}
	s.mu.Unlock()
}

func (s *State) JoinVideoChat() fyne.CanvasObject {
	fmt.Println("jvc")
	var videoPad fyne.CanvasObject
	s.mu.RLock()
	sc, ps := s.SelectedChat, s.ChatPeersShuffled
	s.mu.RUnlock()
	if sc != nil && ps != nil {
		fmt.Println("jvc: selected")
		s.mu.Lock()
		{
			videoPad, s.Videos = CreateVideoPad(ps)
			for peerID, vw := range s.Videos {
				if contact, ok := s.Contacts[peerID]; ok {
					vw.label.SetText(contact.Alias)
				} else {
					vw.label.SetText(peerID)
				}
			}
		}
		s.mu.Unlock()
		s.StartStream(s.Videos[s.Node.Host.ID().String()])
		for _, peer := range ps {
			if peer != s.Node.Host.ID().String() {
				s.RequestStream(peer)
			}
		}
		fmt.Println("jvc: done")
	}
	return videoPad
}

func (s *State) RequestStream(peerID string) {
	s.mu.RLock()
	{
		if safeStream, ok := s.PeerStreamWriters[peerID]; ok {
			safeStream.mu.Lock()
			{
				err := safeStream.stream.WriteMsg(&pb.DataPacket{
					Msg: &pb.DataPacket_StreamInfoResponse{
						StreamInfoResponse: &pb.StreamInfoResponse{
							Answer: pb.StreamInfoResponse_ENTER,
							ChatId: s.SelectedChat.ID,
						},
					},
				})
				if err != nil {
					fmt.Printf("error marshalling or sending STREAM INFO RESPONSE : ENTER [%v]\n", err)
				}
			}
			safeStream.mu.Unlock()
		}
	}
	s.mu.RUnlock()
}

func (s *State) StartStream(preview *VideoWidget) {
	fmt.Println("start stream")
	if s.SelectedChat != nil {
		//start encoding and preview of self

		{
			packetProducer, pw := io.Pipe()
			go func() {
				buf := make([]byte, 1024*50)
				for {
					n, err := packetProducer.Read(buf)
					thisIsInit := false
					if err != nil {
						fmt.Printf("error reading webm chunk: %v\n", err)
						return
					}
					s.mu.Lock()
					{
						if s.InitChunk == nil {
							thisIsInit = true
							s.InitChunk = append(s.InitChunk, buf[:n]...)
						}
					}
					s.mu.Unlock()
					s.mu.RLock()
					{
						for peerID := range s.OutgoingStreams {
							if safeStream, ok := s.PeerStreamWriters[peerID]; ok {
								safeStream.mu.Lock()
								{
									err := safeStream.stream.WriteMsg(&pb.DataPacket{
										Msg: &pb.DataPacket_StreamChunk{
											StreamChunk: &pb.StreamChunk{
												IsInit:    thisIsInit,
												ChatId:    s.SelectedChat.ID,
												SeqNumber: uint32(s.SequenceNumber),
												Data:      buf[:n],
											},
										},
									})
									if err != nil {
										fmt.Printf("error marshalling or sending STREAM CHUNK [%v]\n", err)
									}
								}
								safeStream.mu.Unlock()
							}
						}
					}
					s.mu.RUnlock()
				}
			}()
			fmt.Println("start stream: broadcaster set up")
			videoConsumer, audioConsumer := CreateEncoder(pw)
			fmt.Println("start stream: encoder created")

			startTime := time.Now()

			videoTrack, audioTrack := GetCameraTracks()
			fmt.Printf("videotrack is %v", videoTrack)
			videoTrack.Transform(video.TransformFunc(func(r video.Reader) video.Reader {
				return video.ReaderFunc(func() (img image.Image, release func(), err error) {
					s.VideoMutex.RLock()
					vidOn := s.VideoOn
					s.VideoMutex.RUnlock()
					image, release, error := r.Read()
					if vidOn {
						return image, release, error
					} else {
						release()
						return blackImg, func() {}, nil
					}
				})
			}))
			audioTrack.Transform(audio.TransformFunc(func(r audio.Reader) audio.Reader {
				return audio.ReaderFunc(func() (chunk wave.Audio, release func(), err error) {
					s.AudioMutex.RLock()
					auOn := s.AudioOn
					s.AudioMutex.RUnlock()
					chunk, release, error := r.Read()
					if auOn {
						return chunk, release, error
					} else {
						var silence wave.Audio
						switch v := chunk.(type) {
						case *wave.Float32Interleaved:
							silence = &wave.Float32Interleaved{
								Data: make([]float32, len(v.Data)),
								Size: v.Size,
							}
						case *wave.Float32NonInterleaved:
							newData := make([][]float32, len(v.Data))
							for c := range v.Data {
								newData[c] = make([]float32, len(v.Data[c]))
							}
							silence = &wave.Float32NonInterleaved{
								Data: newData,
								Size: v.Size,
							}
						case *wave.Int16Interleaved:
							silence = &wave.Int16Interleaved{
								Data: make([]int16, len(v.Data)),
								Size: v.Size,
							}
						case *wave.Int16NonInterleaved:
							newData := make([][]int16, len(v.Data))
							for c := range v.Data {
								newData[c] = make([]int16, len(v.Data[c]))
							}
							silence = &wave.Int16NonInterleaved{
								Data: newData,
								Size: v.Size,
							}
						default:
							panic(fmt.Sprintf("unexpected wave.Audio: %#v", v))
						}
						release()
						return silence, func() {}, nil
					}
				})
			}))
			rawVi := videoTrack.NewReader(false)
			encVid, _ := videoTrack.NewEncodedReader("vp8")
			encAud, _ := audioTrack.NewEncodedReader("opus")

			s.mu.Lock()
			s.StreamActive = true
			s.mu.Unlock()

			go func() {
				for {
					s.mu.RLock()
					releaseCamera := !s.StreamActive
					{
						img, release, err := rawVi.Read()
						if err != nil {
							fmt.Printf("error in own-video loop: %v\n", err)
							return
						}
						fyne.Do(func() {
							preview.UpdateFrame(img)
							preview.Refresh()
						})
						release()

						ts := time.Since(startTime).Milliseconds()
						encodedVideo, _, _ := encVid.Read()
						videoConsumer.Write(len(encodedVideo.Data) >= 3 && encodedVideo.Data[0]&0x1 == 0x1, ts, encodedVideo.Data)

						encodedAudio, _, _ := encAud.Read()
						audioConsumer.Write(true, ts, encodedAudio.Data)
					}
					s.mu.RUnlock()
					if releaseCamera {
						fmt.Println("closing camera sequence")
						videoConsumer.Close()
						audioConsumer.Close()
						videoTrack.Close()
						audioTrack.Close()
						break
					}
					time.Sleep(1 * time.Millisecond)
				}
			}()
		}
		fmt.Println("start stream: preliminary done")

		s.mu.RLock()
		{
			pbMsg := &pb.DataPacket{
				Msg: &pb.DataPacket_StreamInfo{
					StreamInfo: &pb.StreamInfo{
						Status: pb.StreamInfo_ACTIVE,
						ChatId: s.SelectedChat.ID,
					},
				},
			}
			for _, peerID := range s.SelectedChat.Peers {
				if peerID != s.Node.Host.ID().String() {
					if safeStream, ok := s.PeerStreamWriters[peerID]; ok {
						safeStream.mu.Lock()
						err := safeStream.stream.WriteMsg(pbMsg)
						if err != nil {
							fmt.Printf("error marshalling or sending STREAM INFO : ACTIVE [%v]\n", err)
						}
						safeStream.mu.Unlock()
					}
					// sendMessageToPeer(peerID, ACTIVE, s.SelectedChat.ID)
				}
			}
		}
		s.mu.RUnlock()
	}
}

func (s *State) LeaveVideoChat() {
	s.mu.RLock()
	sc, peers := s.SelectedChat, s.SelectedChat.Peers
	s.mu.RUnlock()
	if sc != nil {
		s.EndOwnStream()
		for _, peer := range peers {
			if peer != s.Node.Host.ID().String() {
				s.LeaveStream(peer)
			}
		}
	}
}

func (s *State) LeaveStream(peerID string) {
	s.mu.RLock()
	{
		if safeStream, ok := s.PeerStreamWriters[peerID]; ok {
			safeStream.mu.Lock()
			{
				err := safeStream.stream.WriteMsg(&pb.DataPacket{
					Msg: &pb.DataPacket_StreamInfoResponse{
						StreamInfoResponse: &pb.StreamInfoResponse{
							Answer: pb.StreamInfoResponse_LEAVE,
							ChatId: s.SelectedChat.ID,
						},
					},
				})
				if err != nil {
					fmt.Printf("error marshalling or sending STREAM INFO RESPONSE : LEAVE [%v]\n", err)
				}
			}
			safeStream.mu.Unlock()
		}
	}
	s.mu.RUnlock()
	// sendMessageToPeer(peerID, LEAVE)
	s.mu.Lock()
	{
		close(s.IncomingStreams[peerID])
		delete(s.IncomingStreams, peerID)
	}
	s.mu.Unlock()
}

func (s *State) EndOwnStream() {
	s.mu.Lock()
	{
		s.StreamActive = false
	}
	s.mu.Unlock()

	s.mu.RLock()
	{
		pbMsg := &pb.DataPacket{
			Msg: &pb.DataPacket_StreamInfo{
				StreamInfo: &pb.StreamInfo{
					Status: pb.StreamInfo_STOP,
					ChatId: s.SelectedChat.ID,
				},
			},
		}
		for _, peerID := range s.SelectedChat.Peers {
			if peerID != s.Node.Host.ID().String() {
				if safeStream, ok := s.PeerStreamWriters[peerID]; ok {
					safeStream.mu.Lock()
					{
						err := safeStream.stream.WriteMsg(pbMsg)
						if err != nil {
							fmt.Printf("error marshalling or sending STREAM INFO : STOP [%v]\n", err)
						}
					}
					safeStream.mu.Unlock()
				}
				s.mu.Lock()
				{
					delete(s.OutgoingStreams, peerID)
				}
				s.mu.Unlock()
				// sendMessageToPeer(peerID, STOP)
			}
		}
	}
	s.mu.RUnlock()
}
