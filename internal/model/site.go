package model

import "time"

// Site represents a website registered by a user.
// Token is the value embedded in the tracking script's data-site attribute.
type Site struct {
	ID        string
	OwnerID   string
	Name      string
	Domain    string
	Token     string
	Timezone  string
	CreatedAt time.Time
}
