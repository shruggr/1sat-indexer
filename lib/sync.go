package lib

import "github.com/libsv/go-bt/v2"

type Msg struct {
	Id          string
	Height      uint32
	Status      uint32
	Idx         uint32
	Transaction []byte
}

type TxnStatus struct {
	ID       string
	Tx       *bt.Tx
	Height   uint32
	Idx      uint32
	Parents  map[string]*TxnStatus
	Children map[string]*TxnStatus
}
