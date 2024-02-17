/*
	IsaacPaperServer server program
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
	"IsaacPaperServer/Isaac"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"
)

var AdminPswd = flag.String("p", "", "REQUIRED, server admin password")
var TcpAddr = flag.String("t", "0.0.0.0:8555", "server tcp4 address/port, as well as admin port")
var UdpAddr = flag.String("u", "0.0.0.0:8554", "server udp address/port, for p2p gameplay")

func PrintUsage(_ string) error {
	_, _ = fmt.Fprintln(flag.CommandLine.Output(), "Command line argument:")
	flag.PrintDefaults()
	_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage example:\n\t%s -p admin_password\n", os.Args[0])
	os.Exit(1)
	return nil
}

func main() {
	flag.BoolFunc("h", "print the help message", PrintUsage)
	flag.Parse()

	if *AdminPswd == "" {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "Can't start server: Missing password argument.")
		_ = PrintUsage("")
	}

	log.Print("Server protocol version: ", Isaac.PROTOCOL_VER)
	log.Printf("Server started, listening tcp %s and udp %s...", *TcpAddr, *UdpAddr)

	Isaac.LOGIN_PRIVILEDGE = *AdminPswd

	if re, err := regexp.Compile("^$"); err == nil {
		Isaac.TextFilterRe = re
	}
	go Isaac.ServeUdp(*UdpAddr)

	go func() {
		for {
			time.Sleep(time.Minute * 10)
			n := Isaac.DeleteOldLobbies()
			if n != 0 {
				log.Fatal("Delete ", n, " old lobbies.")
			}
		}
	}()

	Isaac.ServeForever("tcp4", *TcpAddr)
}
