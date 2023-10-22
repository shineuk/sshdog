// Copyright 2016 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// TODO: High-level file comment.
package main

import (
	"fmt"
	"github.com/GeertJohan/go.rice"
	"github.com/shineuk/sshdog/daemon"
	"os"
	"strconv"
	"strings"
)

type Debugger bool

func (d Debugger) Debug(format string, args ...interface{}) {
	if d {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg)
	}
}

var dbg Debugger = true

// Lookup the port number
func getPort(box *rice.Box) int16 {
	if len(os.Args) > 1 {
		if port, err := strconv.Atoi(os.Args[1]); err != nil {
			dbg.Debug("Error parsing %s as port: %v", os.Args[1], err)
		} else {
			return int16(port)
		}
	}
	if portData, err := box.String("port"); err == nil {
		portData = strings.TrimSpace(portData)
		if port, err := strconv.Atoi(portData); err != nil {
			dbg.Debug("Error parsing %s as port: %v", portData, err)
		} else {
			return int16(port)
		}
	}
	return 2222 // default
}

// Just check if a file exists
func fileExists(box *rice.Box, name string) bool {
	_, err := box.Bytes(name)
	return err == nil
}

// Should we daemonize?
func shouldDaemonize(box *rice.Box) bool {
	return fileExists(box, "daemon")
}

// Should we be silent?
func beQuiet(box *rice.Box) bool {
	return fileExists(box, "quiet")
}

var mainBox *rice.Box

func main() {
	mainBox = mustFindBox()

	if beQuiet(mainBox) {
		dbg = false
	}

	if shouldDaemonize(mainBox) {
		if err := daemon.Daemonize(daemonStart); err != nil {
			dbg.Debug("Error daemonizing: %v", err)
		}
	} else {
		waitFunc, _ := daemonStart()
		if waitFunc != nil {
			waitFunc()
		}
	}
}

func mustFindBox() *rice.Box {
	// Overloading name 'rice' due to bug in rice to be fixed in 2.0:
	// https://github.com/GeertJohan/go.rice/issues/58
	rice := &rice.Config{
		LocateOrder: []rice.LocateMethod{
			rice.LocateAppended,
			rice.LocateEmbedded,
			rice.LocateWorkingDirectory,
		},
	}
	if box, err := rice.FindBox("config"); err != nil {
		panic(err)
	} else {
		return box
	}
}

// Actually run the implementation of the daemon
func daemonStart() (waitFunc func(), stopFunc func()) {
	server := NewServer()

	hasHostKeys := false
	for _, keyName := range keyNames {
		if keyData, err := mainBox.Bytes(keyName); err == nil {
			dbg.Debug("Adding hostkey file: %s", keyName)
			if err = server.AddHostkey(keyData); err != nil {
				dbg.Debug("Error adding public key: %v", err)
			}
			hasHostKeys = true
		}
	}
	if !hasHostKeys {
		if err := server.RandomHostkey(); err != nil {
			dbg.Debug("Error adding random hostkey: %v", err)
			return
		}
	}

	if authData, err := mainBox.Bytes("authorized_keys"); err == nil {
		dbg.Debug("Adding authorized_keys.")
		server.AddAuthorizedKeys(authData)
	} else {
		dbg.Debug("No authorized keys found: %v", err)
		return
	}
	server.ListenAndServe(getPort(mainBox))
	return server.Wait, server.Stop
}
