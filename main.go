package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync/atomic"

	"golang.org/x/net/websocket"
)

func htmlHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, `
<html>
<head>

</head>

<body>
	<button onclick="connect()">Connect</button>
</body>

<script>
	window.connect = () => {
		document.body.innerHTML = ''
		const url = `+"`"+`${window.location.protocol === 'http:' ? 'ws' : 'wss'}://${window.location.hostname}:${window.location.port}/websocket`+"`"+`
		const ws = new WebSocket(url)

		ws.onmessage = msg => {
			const pc = new RTCPeerConnection()

			pc.onicecandidate = event => {
				if (event.candidate === null) {
					ws.send(pc.currentLocalDescription.sdp)
				}
			}

			let added = false
			pc.ontrack = function (event) {
				if (added) {
					return
				}
				added = true

				const el = document.createElement('video')
				el.srcObject = event.streams[0]
				el.autoplay = true
				el.controls = true

				document.body.appendChild(el)
		  		event.track.onmute = function(event) {
		  		  el.parentNode.removeChild(el)
		  		}
			}

			pc.setRemoteDescription({sdp: msg.data, type: 'offer'})
			pc.createAnswer().then(answer => {
				pc.setLocalDescription(answer)
			})
		}
	}
</script>

</html>
`)
}

var (
	answerChan       chan string
	currentWebsocket atomic.Value
	nilWs            *websocket.Conn
)

func whipHandler(w http.ResponseWriter, req *http.Request) {
	log.Println("Accepted WHIP Request")
	offer, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		panic(err)
	}

	ws, ok := currentWebsocket.Load().(*websocket.Conn)
	if !ok || ws == nil {
		w.WriteHeader(http.StatusInternalServerError)
		panic("WHIP Offer received with no WebSocket client connected")
	}

	if err := websocket.Message.Send(ws, string(offer)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		panic(err)
	}

	w.Header().Set("content-type", "application/sdp")
	w.Header().Set("location", "/")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, <-answerChan)
}

func websocketHandler(ws *websocket.Conn) {
	log.Println("WebSocket connected")
	currentWebsocket.Store(ws)

	for {
		var answer string
		if err := websocket.Message.Receive(ws, &answer); err != nil {
			break
		}

		answerChan <- answer
	}

	log.Println("WebSocket disconnected")
	currentWebsocket.Store(nilWs)
}

func main() {
	currentWebsocket.Store(nilWs)
	answerChan = make(chan string)

	http.HandleFunc("/", htmlHandler)
	http.HandleFunc("/whip", whipHandler)
	http.Handle("/websocket", websocket.Handler(func(ws *websocket.Conn) {
		websocketHandler(ws)
	}))

	httpAddr := os.Getenv("HTTP_ADDR")
	if len(httpAddr) == 0 {
		httpAddr = ":80"
	}

	log.Printf("Starting HTTP Server on %s", httpAddr)
	panic(http.ListenAndServe(httpAddr, nil))
}
