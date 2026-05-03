package domain

import "time"

type RequestStatus string

const (
	RequestStatusPending  RequestStatus = "PENDING"
	RequestStatusApproved RequestStatus = "APPROVED"
	RequestStatusRejected RequestStatus = "REJECTED"
)

type VacationRequest struct {
	ID               int64
	GuildID          string
	UserID           string
	Days             int
	Reason           string
	Status           RequestStatus
	OfficerMessageID string
	OfficerChannelID string
	CreatedAt        time.Time
	DecidedBy        string
	DecidedAt        *time.Time
}
