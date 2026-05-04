package domain

import "time"

type VacationStatus string

const (
	VacationStatusActive VacationStatus = "ACTIVE"
	VacationStatusEnded  VacationStatus = "ENDED"
)

type VacationEndType string

const (
	VacationEndTypeEarlyUser   VacationEndType = "EARLY_USER"
	VacationEndTypeAutoExpired VacationEndType = "AUTO_EXPIRED"
)

type Vacation struct {
	ID            int64
	RequestID     int64
	GuildID       string
	UserID        string
	RoleID        string
	Days          int
	Reason        string
	Status        VacationStatus
	StartedAt     time.Time
	ExpectedEndAt time.Time
	EndedAt       *time.Time
	EndedBy       string
	EndType       VacationEndType
	DMMessageID   string
}

type ActiveVacationView struct {
	ID            int64
	RequestID     int64
	GuildID       string
	UserID        string
	Days          int
	Reason        string
	StartedAt     time.Time
	ExpectedEndAt time.Time
	Status        VacationStatus
}
