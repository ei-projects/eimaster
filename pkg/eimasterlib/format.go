package eimasterlib

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const eiProtoMagic uint32 = 0xDEC0AD07
const eiNicksMagic uint32 = 0xDEADBEEF

// EIServerAddr represents server address. Maybe it makes sense to get rid of this
// and use net.Addr instead
type EIServerAddr struct {
	Family uint16
	Data   [14]uint8
}

type EIGameInfo struct {
	ClientID        uint32
	MasterToken     uint32
	Name            string
	Quest           string
	PlayersCount    uint8
	MaxPlayersCount uint8
	HasPassword     bool
	AllodIndex      uint8
	PlayerNames     []string
}

type EIServerInfo struct {
	Addr net.UDPAddr
	EIGameInfo
	AppearTime time.Time
	LastUpdate time.Time
	Ping       int
}

func NewEIServerAddr(addr *net.UDPAddr) (eiAddr *EIServerAddr, err error) {
	ip := addr.IP.To4()
	if ip == nil || addr.Port < 0 || addr.Port > 65535 {
		return nil, fmt.Errorf("UDPAddr %s is invalid or not supported", addr)
	}

	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint16(addr.Port))
	eiAddr = &EIServerAddr{
		Family: 2,
		Data:   [14]byte{buf.Bytes()[0], buf.Bytes()[1], ip[0], ip[1], ip[2], ip[3]},
	}
	return
}

func (addr *EIServerAddr) Read(r io.Reader) error {
	if err := readLE(r, &addr.Family); err != nil {
		return err
	}
	return readLE(r, &addr.Data)
}

func (addr *EIServerAddr) Write(w io.Writer) error {
	if err := writeLE(w, &addr.Family); err != nil {
		return err
	}
	return writeLE(w, &addr.Data)
}

func (addr *EIServerAddr) String() string {
	return addr.GetUDPAddr().String()
}

func (addr *EIServerAddr) GetUDPAddr() *net.UDPAddr {
	if addr.Family == 2 {
		ip := net.IPv4(addr.Data[2], addr.Data[3], addr.Data[4], addr.Data[5])
		port := binary.BigEndian.Uint16(addr.Data[0:])
		return &net.UDPAddr{IP: ip, Port: int(port)}
	}
	return nil
}

// Copy makes a full copy of the this EIGameInfo struct
func (game *EIGameInfo) Copy() *EIGameInfo {
	result := *game
	result.PlayerNames = make([]string, len(game.PlayerNames))
	copy(result.PlayerNames, game.PlayerNames)
	return &result
}

// IsSentByOrigGame returns true if this game info looks like produced by original game
func (game *EIGameInfo) IsSentByOrigGame() bool {
	// Modified master server protocol always have nicks (maybe len=0, but not nil)
	return game.PlayerNames == nil
}

// Copy makes a full copy of this EIServerInfo struct
func (srv *EIServerInfo) Copy() *EIServerInfo {
	result := *srv
	result.EIGameInfo = *result.EIGameInfo.Copy()
	return &result
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (srv *EIServerInfo) IP() string {
	host, _, err := net.SplitHostPort(srv.Addr.String())
	if err != nil {
		return ""
	}
	return host
}

func (srv *EIServerInfo) String() string {
	return fmt.Sprintf("Name: %q, Addr: %s, ClientID: %08X", srv.Name, srv.Addr.String(), srv.ClientID)
}

func (srv *EIServerInfo) Equals(srv2 *EIServerInfo) bool {
	sameIP := bool2int(srv.IP() == srv2.IP())
	sameClientID := bool2int(srv.ClientID == srv2.ClientID)
	sameParams := bool2int(srv.Name == srv2.Name &&
		srv.AllodIndex == srv2.AllodIndex &&
		srv.MaxPlayersCount == srv2.MaxPlayersCount)
	// At least 2 criteries must be the same
	return sameIP+sameClientID+sameParams >= 2
}

func readLength(r io.Reader, length *int) error {
	var err error
	var codedLen [4]uint8
	checkErr(&err, readFull(r, codedLen[0:1]))
	if codedLen[0]%2 == 1 {
		checkErrNoEOF(&err, readFull(r, codedLen[1:4]))
	}
	if err != nil {
		return err
	}
	*length = int(binary.LittleEndian.Uint32(codedLen[:]) / 2)
	return nil
}

func writeLength(w io.Writer, length int) error {
	if length <= 127 {
		return writeLE(w, uint8(length)*2)
	}
	return writeLE(w, uint32(length)*2+1)
}

func readString(r io.Reader, str *string) error {
	var err error
	var length int
	if err = readLength(r, &length); err != nil {
		return err
	}
	if length > 1024*1024 {
		return errors.New("length is too big")
	}
	if length == 0 {
		*str = ""
		return nil
	}
	data := make([]byte, length)
	err = readFull(r, data)
	*str = DecodeWin1251(data)
	return err
}

func writeString(w io.Writer, str string) error {
	encoded := EncodeWin1251(str)
	if err := writeLength(w, len(encoded)); err != nil {
		return err
	}
	_, err := w.Write(encoded)
	return err
}

func readPlayerNames(r io.Reader, res *[]string) error {
	var err error
	var namesCount uint8
	if err = readByte(r, &namesCount); err != nil {
		return err
	}

	namesData := make([]byte, 0, 128)
	names := make([]string, namesCount)
	for i := range names {
		var nameLen uint8
		checkErrNoEOF(&err, readLE(r, &nameLen))
		namesData = namesData[0:0:cap(namesData)]
		for ; nameLen > 0 && err == nil; nameLen-- {
			var b byte
			checkErrNoEOF(&err, readByte(r, &b))
			if b != 0 { // Skip zero bytes
				namesData = append(namesData, b)
			}
		}
		if err != nil {
			return err
		}
		names[i] = DecodeWin1251(namesData)
	}
	*res = names
	return nil
}

func writePlayerNames(w io.Writer, names []string) error {
	if err := writeIntAsUint8(w, len(names)); err != nil {
		return err
	}
	for _, name := range names {
		encoded := EncodeWin1251(name)
		if err := writeIntAsUint8(w, len(encoded)); err != nil {
			return err
		}
		if _, err := w.Write(encoded); err != nil {
			return err
		}
	}
	return nil
}

func ReadGameInfo(r io.Reader, full bool, game *EIGameInfo) error {
	var err error
	var protoMagic uint32
	var nicksMagic uint32 = eiNicksMagic
	checkErr(&err, readLE(r, &game.ClientID))
	checkErrNoEOF(&err, readLE(r, &game.MasterToken))
	checkErrNoEOF(&err, readString(r, &game.Name))
	checkErrNoEOF(&err, readString(r, &game.Quest))
	checkErrNoEOF(&err, readLE(r, &game.PlayersCount))
	checkErrNoEOF(&err, readLE(r, &game.MaxPlayersCount))
	checkErrNoEOF(&err, readLE(r, &game.HasPassword))
	checkErrNoEOF(&err, readLE(r, &game.AllodIndex))
	checkErrNoEOF(&err, readLE(r, &protoMagic))
	if full {
		checkErrNoEOF(&err, readLE(r, &nicksMagic))
		checkErrNoEOF(&err, readPlayerNames(r, &game.PlayerNames))
	}
	if err == nil && protoMagic != eiProtoMagic {
		err = fmt.Errorf("invalid proto magic: expected %08X got %08X", eiProtoMagic, protoMagic)
	}
	if err == nil && nicksMagic != eiNicksMagic {
		err = fmt.Errorf("invalid nicks magic: expected %08X got %08X", eiNicksMagic, nicksMagic)
	}
	return err
}

func WriteGameInfo(w io.Writer, full bool, game *EIGameInfo) error {
	var err error
	checkErr(&err, writeLE(w, game.ClientID))
	checkErr(&err, writeLE(w, game.MasterToken))
	checkErr(&err, writeString(w, game.Name))
	checkErr(&err, writeString(w, game.Quest))
	checkErr(&err, writeLE(w, game.PlayersCount))
	checkErr(&err, writeLE(w, game.MaxPlayersCount))
	checkErr(&err, writeLE(w, game.HasPassword))
	checkErr(&err, writeLE(w, game.AllodIndex))
	checkErr(&err, writeLE(w, eiProtoMagic))
	if full {
		checkErr(&err, writeLE(w, eiNicksMagic))
		checkErr(&err, writePlayerNames(w, game.PlayerNames))
	}
	return err
}

func ReadMasterResponse(r io.Reader, game *EIGameInfo) error {
	var err error
	var magic byte
	var clientID uint32
	var masterToken uint32
	checkErr(&err, readByte(r, &magic))
	checkErrNoEOF(&err, readLE(r, &clientID))
	checkErrNoEOF(&err, readLE(r, &masterToken))
	if err == nil && magic != 0xFF {
		err = fmt.Errorf("invalid magic: expected FF got %02X", magic)
	}
	if err == nil && clientID != game.ClientID {
		err = fmt.Errorf("invalid client id: expected %08X got %08X", game.ClientID, clientID)
	}
	if err == nil {
		game.MasterToken = masterToken
	}
	return err
}

func WriteMasterResponse(w io.Writer, game *EIGameInfo) error {
	var err error
	checkErr(&err, writeIntAsUint8(w, 0xFF))
	checkErr(&err, writeLE(w, game.ClientID))
	checkErr(&err, writeLE(w, game.MasterToken))
	return err
}

func ReadServerInfo(r io.Reader, full bool, srv *EIServerInfo) error {
	var err error
	var eiAddr EIServerAddr
	checkErr(&err, eiAddr.Read(r))
	checkErrNoEOF(&err, ReadGameInfo(r, full, &srv.EIGameInfo))
	if err == nil {
		if udpAddr := eiAddr.GetUDPAddr(); udpAddr != nil {
			srv.Addr = *udpAddr
		} else {
			err = errors.New("cannot parse server address")
		}
	}
	return err
}

func WriteServerInfo(w io.Writer, full bool, srv *EIServerInfo) error {
	var err error
	eiAddr, err := NewEIServerAddr(&srv.Addr)
	if err != nil {
		return fmt.Errorf("cannot serialize server address: %w", err)
	}
	checkErr(&err, eiAddr.Write(w))
	checkErr(&err, WriteGameInfo(w, full, &srv.EIGameInfo))
	return err
}

func ReadServersList(r io.Reader, full bool, res *[]EIServerInfo) error {
	var err error
	var srv EIServerInfo
	servers := make([]EIServerInfo, 0, 16)
	for {
		if err = ReadServerInfo(r, full, &srv); err != nil {
			break
		}
		servers = append(servers, srv)
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}
	*res = servers
	return err
}

func WriteServersList(w io.Writer, full bool, servers []EIServerInfo) error {
	for _, server := range servers {
		if err := WriteServerInfo(w, full, &server); err != nil {
			return err
		}
	}
	return nil
}
