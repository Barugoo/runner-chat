package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/Barugoo/twaiv/runner-chat/internal/notifier"
	"github.com/gorilla/websocket"
)

func main() {
	port := flag.String("p", "", "port of notifier")
	flag.Parse()

	if *port == "" {
		log.Fatal("flag -port is not specified")
	}

	var upgrader websocket.Upgrader

	notifier, _ := notifier.NewNotifier(3)

	http.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		type req struct {
			UserID       string
			DeviceTokens []string
			Events       []any
		}

		var body []req

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			log.Println(err)
			http.Error(w, "unable to decode body", http.StatusInternalServerError)
			return
		}

		for _, batchItem := range body {
			for _, e := range batchItem.Events {
				if err := notifier.BroadcastEvent(r.Context(), batchItem.UserID, batchItem.DeviceTokens, e); err != nil {
					log.Println(err)
					http.Error(w, "unable to notify user", http.StatusInternalServerError)
					return
				}
			}
		}
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		defer conn.Close()

		userID := r.URL.Query().Get("user_id")
		deviceToken := r.URL.Query().Get("device_token")

		wsEvents := notifier.RegisterUser(r.Context(), userID, deviceToken)
		defer notifier.UnregisterUser(r.Context(), userID, deviceToken)

		for {
			select {
			case e := <-wsEvents:
				if err := conn.WriteJSON(e); err != nil {
					log.Println(err)
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	})

	log.Println("running notifier on port", *port)
	log.Fatal(http.ListenAndServe("0.0.0.0:"+*port, nil))
}
