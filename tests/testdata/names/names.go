package names

// Record and Partial would shadow the TypeScript utility types of the same name.
type Record struct {
	ID int `json:"id"`
}

type Partial struct {
	Done bool `json:"done"`
}

type K string

type Table struct {
	ByName map[string]Record `json:"byName"`
	ByID   map[int]Partial   `json:"byId"`
	ByKey  map[K]int         `json:"byKey"`
}
