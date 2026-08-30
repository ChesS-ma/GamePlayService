package http

import "net/http"

// NewRouter wires every inbound route. main and the integration tests both use
// it, so the tests exercise the same routing the server actually serves.
func NewRouter(game *GameHandler, ws *WsHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/games/create", game.CreateGame)
	mux.HandleFunc("/games/get", game.GetGame)
	// Moves travel over the socket during play; the REST route exists for
	// tooling and tests that do not want a connection.
	mux.HandleFunc("/games/move", game.MakeMove)
	mux.HandleFunc("/ws", ws.HandleWS)
	return mux
}
