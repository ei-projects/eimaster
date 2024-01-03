package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	golog "log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	master "github.com/ei-projects/eimaster/pkg/eimasterlib"
	"github.com/ei-projects/eimaster/pkg/lzevil"
	"github.com/spf13/cobra"
)

var (
	serverMainAddr   = "0.0.0.0:28004"
	serverHttpAddr   = ""
	serverHttpPrefix = "/"
)

var runCmd = cobra.Command{
	Use:   "run",
	Short: "Run master server",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		serverMainAddr, _ = cmd.Flags().GetString("addr")
		serverHttpAddr, _ = cmd.Flags().GetString("http-addr")
		serverHttpPrefix, _ = cmd.Flags().GetString("http-prefix")
		serverMainLoop()
	},
}
var serverCmds = []*cobra.Command{&runCmd}

var (
	servUpdates = make(chan *master.EIServerInfo, 100)
	servLists   = make(chan []master.EIServerInfo)
)

func init() {
	runCmd.Flags().String("addr", serverMainAddr, "Set server address")
	runCmd.Flags().String("http-addr", serverHttpAddr,
		"Set http server address. Don't serve if not set or empty")
	runCmd.Flags().String("http-prefix", serverHttpPrefix, "Prefix for http servers")
}

func handleServerInfo(pc net.PacketConn, addr net.Addr, data []byte) {
	log.Debugf("Data recieved %d bytes from %s", len(data), addr)

	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok || udpAddr == nil {
		log.Errorf("Failed to cast addr %s to UDPAddr", addr)
	}

	srv := master.EIServerInfo{
		Addr:       *udpAddr,
		AppearTime: time.Now(),
		LastUpdate: time.Now(),
	}

	r := bytes.NewReader(data)
	err := master.ReadGameInfo(r, false, &srv.EIGameInfo)
	if err != nil {
		log.Errorf("Failed to parse game info: %s, hex dump:\n%s",
			err.Error(), getHexDump(data))
		return
	}
	if r.Len() > 0 {
		// Some data remaining. Probably it's nicks
		r.Reset(data)
		err = master.ReadGameInfo(r, true, &srv.EIGameInfo)
		if err != nil {
			log.Errorf("Failed to parse game info: %s, hex dump:\n%s",
				err.Error(), getHexDump(data))
			return
		}
	}

	jsonData, _ := json.Marshal(&srv)
	log.Infof("Received game from: %s", string(jsonData))

	if srv.MasterToken == 0 && srv.IsSentByOrigGame() {
		srv.MasterToken = 1 + uint32(rand.Int63n(0xFFFFFFFE))
		log.Debugf("Sending password %08X to %s", srv.MasterToken, &srv)
		var buf bytes.Buffer
		master.WriteMasterResponse(&buf, &srv.EIGameInfo)
		pc.WriteTo(buf.Bytes(), addr)
	}

	servUpdates <- &srv
}

func serversReciever(ctx context.Context) error {
	pc, err := net.ListenPacket("udp", serverMainAddr)
	if err != nil {
		return fmt.Errorf("failed to listen udp:%s: %w", serverMainAddr, err)
	}
	defer pc.Close()

	log.Infof("Listening on udp:%s", serverMainAddr)

	doneChan := make(chan error, 1)
	go func() {
		buffer := make([]uint8, 4096)
		for {
			n, addr, err := pc.ReadFrom(buffer)
			if err != nil {
				doneChan <- err
				return
			}

			data := make([]byte, n)
			copy(data, buffer[:n])
			go handleServerInfo(pc, addr, data)
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-doneChan:
		return err
	}
}

func sendServersInfo(conn net.Conn) {
	defer conn.Close()

	var data [4]byte
	var clientID uint32
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err := io.ReadFull(conn, data[:])
	if err != nil {
		log.Warnf("Client %s hasn't sent its ID before requesting server list: %s",
			conn.RemoteAddr(), err)
	} else {
		binary.Read(bytes.NewReader(data[:]), binary.LittleEndian, &clientID)
	}

	servList := <-servLists
	log.Infof("Client addr: %s id: %08X connected. Sending %d servers...\n",
		conn.RemoteAddr(), clientID, len(servList))

	// Serialize servers list
	var buf1, buf2 bytes.Buffer
	master.WriteServersList(&buf1, false, servList)
	w := lzevil.NewWriter(&buf2, buf1.Len())
	w.Write(buf1.Bytes())

	buf1.Reset()
	binary.Write(&buf1, binary.LittleEndian, uint32(buf2.Len()+4))
	buf1.Write(buf2.Bytes())

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = io.Copy(conn, &buf1)
	if err != nil {
		log.Warnf("Failed to send servers list to %s: %s", conn.RemoteAddr(), err)
		return
	}

	// The game needs this delay for some reason... Test client works fine without it
	time.Sleep(100 * time.Millisecond)
}

func serversSender(ctx context.Context) error {
	listener, err := net.Listen("tcp", serverMainAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on tcp addr %s: %w", serverMainAddr, err)
	}
	defer listener.Close()

	log.Infof("Listening on tcp:%s", serverMainAddr)

	doneChan := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				doneChan <- err
				return
			}
			go sendServersInfo(conn)
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-doneChan:
		return err
	}
}

func serversSenderJSON(ctx context.Context) error {
	handler := http.NewServeMux()
	handler.HandleFunc(serverHttpPrefix, func(w http.ResponseWriter, req *http.Request) {
		data, err := json.Marshal(<-servLists)
		if err != nil {
			log.Errorf("Failed to convert server list to JSON: %s", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write((data))
		if err != nil {
			log.Errorf("Failed write HTTP response: %s", err)
			return
		}
	})

	logWriter := log.Writer()
	defer logWriter.Close()
	server := http.Server{
		Addr:              serverHttpAddr,
		ErrorLog:          golog.New(logWriter, "", 0),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	doneChan := make(chan error, 1)
	go func() {
		log.Infof("Listening on http:%s", server.Addr)
		doneChan <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		server.Shutdown(context.Background())
		return ctx.Err()
	case err := <-doneChan:
		return err
	}
}

func findServer(target *master.EIServerInfo, servList []*master.EIServerInfo) *master.EIServerInfo {
	for _, srv := range servList {
		if srv.Equals(target) {
			return srv
		}
	}
	return nil
}

func maintainServerList(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Run pinger
	pingsChan := make(chan *master.EIServerInfo, 100)
	pingsUpdates := make(chan *master.EIServerInfo, 100)
	defer close(pingsChan)
	go func() {
		for srv := range pingsChan {
			go func(srv *master.EIServerInfo) {
				pingServer(srv, 2500*time.Millisecond)
				pingsUpdates <- srv
			}(srv)
		}
		for range pingsUpdates {
			// Just consume remaining ping updates
		}
	}()

	servList := make([]*master.EIServerInfo, 0)
	servListToSend := make([]master.EIServerInfo, 0)

	for {
		select {
		case <-ticker.C:
			curTime := time.Now()
			newServList := make([]*master.EIServerInfo, 0, len(servList))
			newServListToSend := make([]master.EIServerInfo, 0, len(servList))
			for _, srv := range servList {
				if curTime.Sub(srv.LastUpdate) > 30*time.Minute {
					log.Debugf("Server %s hasn't sent updates for 30 min, removing...", srv)
				} else if curTime.Sub(srv.LastUpdate) > 2*time.Minute {
					// This server is a candidate to remove. Let's keep it for some time.
					// But don't send it
					newServList = append(newServList, srv)
				} else {
					newServList = append(newServList, srv)
					newServListToSend = append(newServListToSend, *srv.Copy())
					pingsChan <- srv.Copy()
				}
			}
			servList = newServList
			servListToSend = newServListToSend
			log.Debugf("Servers list were refreshed: running: %d, visible: %d...",
				len(servList), len(servListToSend))

		case updSrv := <-servUpdates:
			existingSrv := findServer(updSrv, servList)

			if existingSrv == nil {
				log.Debugf("Received new server: %s", updSrv)
				servList = append(servList, updSrv)
			} else {
				log.Debugf("Received update for existing server: %s", updSrv)
				// Reuse some parameters from existing server
				updSrv.AppearTime = existingSrv.AppearTime
				updSrv.Ping = existingSrv.Ping
				updSrv.LastSuccessfulPing = existingSrv.LastSuccessfulPing
				if time.Since(existingSrv.LastSuccessfulPing) < 2*time.Minute {
					// Keep address from existing server if it was pingable.
					updSrv.Addr = existingSrv.Addr
				}
				if updSrv.IsSentByOrigGame() {
					updSrv.PlayerNames = existingSrv.PlayerNames
				}
				*existingSrv = *updSrv
			}

		case updSrv := <-pingsUpdates:
			existingSrv := findServer(updSrv, servList)
			if existingSrv != nil {
				existingSrv.Ping = updSrv.Ping
				existingSrv.LastSuccessfulPing = updSrv.LastSuccessfulPing
			}

		case servLists <- servListToSend:
			log.Debugf("Sending %d servers", len(servListToSend))

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func serverMainLoop() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := <-c
		log.Infof("Got signal: %v. Stopping server", sig)
		cancel()
	}()

	var wg sync.WaitGroup
	startWorker := func(name string, f func(ctx context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := f(ctx)
			if errors.Is(err, context.Canceled) {
				log.Debugf("%s cancelled", name)
			} else {
				log.Errorf("%s failed: %s", name, err)
				cancel()
			}
		}()
	}

	startWorker("Reciever", serversReciever)
	startWorker("Sender", serversSender)
	if serverHttpAddr != "" {
		startWorker("SenderJSON", serversSenderJSON)
	}
	startWorker("Maintainer", maintainServerList)

	log.Info("Server started")
	<-ctx.Done()

	log.Info("Server is stopping...")
	wg.Wait()

	log.Info("Server stopped")
}
