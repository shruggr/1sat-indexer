package lib

type Claim struct {
	SubDomain string `json:"sub"`
	Type      string `json:"type"`
	Value     string `json:"value"`
}

type OpNS struct {
	Genesis *Outpoint `json:"genesis,omitempty"`
	Domain  string    `json:"domain"`
	Status  int       `json:"status"`
}
