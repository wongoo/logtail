/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vogo/logger"
	"github.com/vogo/logtail"
	"github.com/vogo/logtail/transfer"
)

const WebsocketHeartbeatReadTimeout = 15 * time.Second

// nolint:gochecknoglobals // ignore this
var websocketUpgrader = websocket.Upgrader{}

// nolint:gochecknoglobals // ignore this
var wsConnIndex int64

type WebsocketTransfer struct {
	id   string
	conn *websocket.Conn
}

func (ww *WebsocketTransfer) Name() string {
	return ww.id
}

func (ww *WebsocketTransfer) Start() error { return nil }

func (ww *WebsocketTransfer) Stop() error { return nil }

func (ww *WebsocketTransfer) Trans(_ string, data ...[]byte) (err error) {
	for _, d := range data {
		err = ww.conn.WriteMessage(1, d)
		if err != nil {
			return err
		}
	}

	return nil
}

func startWebsocketTransfer(runner *logtail.Runner, response http.ResponseWriter, request *http.Request, serverID string) {
	wsConn, err := websocketUpgrader.Upgrade(response, request, nil)
	if err != nil {
		logger.Error("web socket error:", err)

		return
	}
	defer wsConn.Close()

	server, ok := runner.Servers[serverID]
	if !ok {
		logger.Warnf("server id not found: %s", serverID)

		return
	}

	websocketTransfer := &WebsocketTransfer{conn: wsConn}
	index := fmt.Sprintf("ww-%d", atomic.AddInt64(&wsConnIndex, 1))
	router := logtail.NewRouter(server, index, nil, []transfer.Transfer{websocketTransfer})
	server.MergingWorker.StartRouterFilter(router)
	startWebsocketHeartbeat(router, websocketTransfer)
}

const MessageTypeMatcherConfig = '1'

func startWebsocketHeartbeat(router *logtail.Router, websocketTransfer *WebsocketTransfer) {
	defer func() {
		_ = recover()

		router.Stop()
		logger.Infof("router [%s] websocket heartbeat stopped", router.Name)
	}()

	for {
		select {
		case <-router.Stopper.C:
			return
		default:
			_ = websocketTransfer.conn.SetReadDeadline(time.Now().Add(WebsocketHeartbeatReadTimeout))

			_, data, err := websocketTransfer.conn.ReadMessage()
			if err != nil {
				if !isEncodeError(err) {
					logger.Warnf("router [%s] websocket heartbeat error: %+v", router.Name, err)
					router.Stop()
				}

				return
			}

			if len(data) > 0 && data[0] == MessageTypeMatcherConfig {
				if configErr := handleMatcherConfigUpdate(router, data[1:]); configErr != nil {
					logger.Warnf("router [%s] websocket matcher config error: %+v", router.Name, configErr)
				}
			}
		}
	}
}

func isEncodeError(err error) bool {
	return strings.Contains(err.Error(), "utf8")
}

func handleMatcherConfigUpdate(router *logtail.Router, data []byte) error {
	var matcherConfigs []*logtail.MatcherConfig
	if err := json.Unmarshal(data, &matcherConfigs); err != nil {
		return err
	}

	matchers, matchErr := logtail.NewMatchers(matcherConfigs)
	if matchErr != nil {
		return matchErr
	}

	router.SetMatchers(matchers)

	return nil
}
