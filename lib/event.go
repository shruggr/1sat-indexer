package lib

type Event struct {
	Vout    uint32 `json:"vout"`
	Channel string `json:"channel"`
	// Event   []byte `json:"event"`
	Data []byte `json:"data"`
}
