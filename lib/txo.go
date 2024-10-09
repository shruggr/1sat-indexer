package lib

type Txo struct {
	Outpoint *Outpoint             `json:"outpoint,omitempty"`
	Satoshis uint64                `json:"satoshis"`
	OutAcc   uint64                `json:"outacc"`
	Owners   []string              `json:"owners,omitempty"`
	Data     map[string]*IndexData `json:"-"`
}

func (t *Txo) AddOwner(owner string) {
	for _, o := range t.Owners {
		if o == owner {
			return
		}
	}
	t.Owners = append(t.Owners, owner)
}
