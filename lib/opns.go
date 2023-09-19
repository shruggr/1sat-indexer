package lib

type Claim struct {
	SubDomain string `json:"sub"`
	Type      string `json:"type"`
	Value     string `json:"value"`
}

type OpNS struct {
	Claims    []*Claim          `json:"claims"`
	ClaimedBy map[string]*Claim `json:"claimedby"`
}
