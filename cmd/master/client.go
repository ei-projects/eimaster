package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ei-projects/eimaster/pkg/eimasterlib"
	"github.com/ei-projects/eimaster/pkg/lzevil"
	"github.com/spf13/cobra"
)

var clientCmds = []*cobra.Command{
	{
		Use:                "get [flags]",
		Short:              "Get list of servers",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			getCommand(args)
		},
	},
	{
		Use:                "send [flags]",
		Short:              "Send a fake server",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			sendCommand(args)
		},
	},
}

func getServers(addr string, nicks bool, verbose bool) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Fatalf("ResolveTCPAddr failed: %s", err.Error())
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		log.Fatalf("DialTCP failed: %s", err.Error())
	}

	err = binary.Write(conn, binary.LittleEndian, uint32(0xDEADBEEF))
	if err != nil {
		log.Fatalf("conn.Write failed: %s", err.Error())
	}

	var inputSize int32
	err = binary.Read(conn, binary.LittleEndian, &inputSize)
	if err != nil || inputSize < 0 || inputSize > 100000 {
		log.Fatalf("conn.Read failed: %s", err.Error())
	}

	lzreader := lzevil.NewReader(conn)
	var servers []eimasterlib.EIServerInfo
	if err := eimasterlib.ReadServersList(lzreader, false, &servers); err != nil {
		log.Fatalf("Fail to read servers: %s", err.Error())
	}

	pingServers(servers)

	fmt.Printf(
		"%-12s %-8s %-6s %-9s %-22v %-6s ",
		"Name", "Players", "Allod", "Password", "Address", "Ping")
	if verbose {
		fmt.Printf("%-9s %-9s ", "#UsrID#", "#UsrToken#")
	}
	fmt.Printf("%s\n", "Quest")
	for _, srv := range servers {
		fmt.Printf("%-12s %-8s %-6d %-9v %-22v %-6d ",
			srv.Name,
			fmt.Sprintf("%d / %d", srv.PlayersCount, srv.MaxPlayersCount),
			srv.AllodIndex,
			srv.HasPassword,
			srv.Addr.String(),
			srv.Ping,
		)
		if verbose {
			fmt.Printf("%-09X %-09X ", srv.ClientID, srv.MasterToken)
		}
		fmt.Printf("%s\n", srv.Quest)
		if nicks {
			for _, name := range srv.PlayerNames {
				println("    ", name)
			}
		}
	}
}

func getCommand(args []string) {
	flagSet := flag.NewFlagSet("get", flag.ExitOnError)
	nicksFlag := flagSet.Bool("nicks", false, "Print player names")
	verboseFlag := flagSet.Bool("verbose", false, "Print verbose information")
	flagSet.Usage = func() {
		println("Usage: eimaster client get [-nicks] [-verbose] [--] <addr>\n")
		println("Get servers list from specified master server\n")
		println("Arguments:")
		flagSet.PrintDefaults()
		println("  addr")
		println("        Master server address\n")
	}

	flagSet.Parse(args)
	if !flagSet.Parsed() {
		log.Fatalln("Invalid arguments for 'get' command. " +
			"Try 'client get -h' for mor information")
	}
	if flagSet.NArg() < 1 {
		log.Fatalln("Not enough arguments for 'get' command. " +
			"Try 'client get -h' for mor information")
	}

	addr := flagSet.Arg(0)

	if !strings.Contains(addr, ":") {
		addr += ":28004"
	}

	getServers(addr, *nicksFlag, *verboseFlag)
}

func sendCommand(args []string) {
	flagSet := flag.NewFlagSet("get", flag.ExitOnError)
	nicksFlag := flagSet.Bool("nicks", false, "Send with player names")
	sourceAddFlag := flagSet.String("saddr", ":0", "Source UDP address")
	flagSet.Usage = func() {
		println("Usage: eimaster client send [-nicks] [--] <addr>\n")
		println("Send a fake server info to specified master server\n")
		println("Arguments:")
		flagSet.PrintDefaults()
		println("  addr")
		println("        Master server address\n")
	}

	flagSet.Parse(args)
	if !flagSet.Parsed() {
		log.Fatalln("Invalid arguments for 'get' command. " +
			"Try 'client get -h' for mor information")
	}
	if flagSet.NArg() < 1 {
		log.Fatalln("Not enough arguments for 'get' command. " +
			"Try 'client get -h' for mor information")
	}

	masterAddr := flagSet.Arg(0)
	if !strings.Contains(masterAddr, ":") {
		masterAddr += ":28004"
	}

	sourceAddr, err := net.ResolveUDPAddr("udp", *sourceAddFlag)
	if err != nil {
		log.Fatalf("Failed to resolve %s addr: %v", *sourceAddFlag, err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", masterAddr)
	if err != nil {
		log.Fatalf("Failed to resolve %s addr: %v", masterAddr, err)
	}

	game := eimasterlib.EIGameInfo{
		ClientID:        0xABBACAFE,
		MasterToken:     0,
		Name:            "Fake server",
		Quest:           "Fake quest",
		PlayersCount:    0,
		MaxPlayersCount: 13,
		HasPassword:     true,
		AllodIndex:      5,
		PlayerNames:     []string{"fake1", "fake2", "fake3"},
	}

	conn, err := net.DialUDP("udp", sourceAddr, udpAddr)
	if err != nil {
		log.Fatalf("net.DialUDP failed: %s", err)
	}

	conn.SetDeadline(time.Now().Add(2000 * time.Millisecond))

	var buf bytes.Buffer
	eimasterlib.WriteGameInfo(&buf, false, &game)
	log.Debugf("Sending fake game:\n%s", getHexDump(buf.Bytes()))

	n, err := conn.Write(buf.Bytes())
	if n < buf.Len() || err != nil {
		log.Fatalf("conn.Write failed: %s. %d bytes were written", err, n)
	}

	recvBuf := make([]byte, 128)
	n, err = conn.Read(recvBuf)
	if n == 0 || err != nil {
		log.Fatalf("Master server hasn't answered: %s, %d", err, n)
	}
	log.Debugf("Response from master:\n%s", getHexDump(recvBuf[:n]))

	err = eimasterlib.ReadMasterResponse(bytes.NewReader(recvBuf), &game)
	if err != nil {
		log.Fatalf("Failed to parse response from master server: %s", err.Error())
	}
	log.Debugf("Received password for master server: %08X", game.MasterToken)

	buf.Reset()
	eimasterlib.WriteGameInfo(&buf, *nicksFlag, &game)
	log.Debugf("Sending fake game again with password:\n%s", getHexDump(buf.Bytes()))
	n, err = conn.Write(buf.Bytes())
	if n < buf.Len() || err != nil {
		log.Fatalf("conn.Write failed: %s. %d bytes were written", err, n)
	}
}
