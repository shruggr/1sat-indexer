package lib

type Txo struct {
	Txid     []byte
	Vout     uint32
	Satoshis uint64
	AccSats  uint64
	Lock     []byte
	Spend    []byte
	Origin   []byte
	Ordinal  uint64
}

func LoadTxo(txid []byte, vout uint32) (txo *Txo, err error) {
	txo = &Txo{}
	err = GetTxo.QueryRow(txid, vout).Scan(
		&txo.Txid,
		&txo.Vout,
		&txo.Satoshis,
		&txo.AccSats,
		&txo.Lock,
		&txo.Spend,
		&txo.Origin,
	)
	if err != nil {
		return nil, err
	}
	return
}

func LoadTxos(txid []byte) (txos []*Txo, err error) {
	rows, err := GetTxos.Query(txid)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		txo := &Txo{}
		err = rows.Scan(
			&txo.Txid,
			&txo.Vout,
			&txo.Satoshis,
			&txo.AccSats,
			&txo.Lock,
			&txo.Spend,
			&txo.Origin,
		)
		if err != nil {
			return
		}
		txos = append(txos, txo)
	}
	return
}
