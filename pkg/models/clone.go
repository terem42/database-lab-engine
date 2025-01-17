/*
2019 © Postgres.ai
*/

package models

// Clone defines a clone model.
type Clone struct {
	ID        string        `json:"id"`
	Snapshot  *Snapshot     `json:"snapshot"`
	Protected bool          `json:"protected"`
	DeleteAt  string        `json:"deleteAt"`
	CreatedAt string        `json:"createdAt"`
	Status    Status        `json:"status"`
	DB        Database      `json:"db"`
	Metadata  CloneMetadata `json:"metadata"`
}

// CloneMetadata contains fields describing a clone model.
type CloneMetadata struct {
	CloneDiffSize   uint64  `json:"cloneDiffSize"`
	CloneDiffSizeHR string  `json:"cloneDiffSizeHR"`
	CloningTime     float64 `json:"cloningTime"`
	MaxIdleMinutes  uint    `json:"maxIdleMinutes"`
}
