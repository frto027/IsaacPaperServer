/*
	IsaacPaperServer admin tools
	Copyright (C) 2024  frto027

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as
	published by the Free Software Foundation, either version 3 of the
	License, or (at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"IsaacPaperServer/0xf7.top/IsaacPaperServer/Isaacpb"
	"IsaacPaperServer/Isaac"
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/fatih/color"
	"google.golang.org/protobuf/proto"
)

func Auth(conn net.Conn) {
	writer := bufio.NewWriter(conn)
	_, err := writer.WriteString(Isaac.LOGIN_PRIVILEDGE)
	if err != nil {
		log.Fatal(err)
	}
	err = writer.WriteByte(0)
	if err != nil {
		log.Fatal(err)
	}
	err = writer.Flush()
	if err != nil {
		log.Fatal(err)
	}
}

func ReadAndDisplay(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		s, err := reader.ReadString(0)
		if err != nil {
			if err == io.EOF {
				log.Print("the the remote connection has been close.")
				os.Exit(0)
			}
			log.Fatal(err)
		}
		for len(s) > 0 && s[len(s)-1] == 0 {
			s = s[:len(s)-1]
		}
		color.Blue("%s", s)
	}
}

func TypeAndSend(conn net.Conn) {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(conn)
	for {
		s, err := reader.ReadString('\n')
		if err != nil {
			os.Exit(0)
		}
		_, err = writer.WriteString(s)
		if err != nil {
			log.Fatal(err)
		}

		err = writer.WriteByte(0)
		if err != nil {
			log.Fatal(err)
		}
		err = writer.Flush()
		if err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage: ", os.Args[0], " <server:port> <admin password>")
		fmt.Println("\texample: ", os.Args[0], "127.0.0.1:8555 123456")
		os.Exit(1)
	}
	//address := "isaacpcp.0xf7.top:8555"
	address := os.Args[1]
	Isaac.LOGIN_PRIVILEDGE = os.Args[2]

	conn, err := net.Dial("tcp", address)
	//conn, err := net.Dial("tcp", "127.0.0.1:8555")
	if err != nil {
		log.Fatal(err)
	}

	header := Isaacpb.RequestHeader{
		Type:      Isaacpb.RequestHeader_AdminLogin,
		HoldValue: 0,
		Length:    0,
	}

	bts, err := proto.Marshal(&header)
	if err != nil {
		log.Fatal(err)
	}

	buff := make([]byte, 4)
	binary.BigEndian.PutUint32(buff, uint32(len(bts)))
	sent := 0
	for sent < 4 {
		s, err := conn.Write(buff[sent:])
		if err != nil {
			log.Fatal(err)
		}
		if s <= 0 {
			log.Fatal("can't send")
		}
		sent += s
	}
	sent = 0
	for sent < len(bts) {
		s, err := conn.Write(bts[sent:])
		if err != nil {
			log.Fatal(err)
		}
		if s <= 0 {
			log.Fatal("can't send")
		}
		sent += s
	}

	go ReadAndDisplay(conn)
	Auth(conn)
	TypeAndSend(conn)
}
