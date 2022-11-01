package ussdapp

import (
	"time"
)

var sessionLogsTable = ""

const defaultSessionsLogsTable = "ussd_logs"

type SessionRequest struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	SessionID     string    `gorm:"index;type:varchar(100);not null"`
	Msisdn        string    `gorm:"index;type:varchar(13);not null"`
	MenuName      string    `gorm:"index;type:varchar(50);not null"`
	USSDParams    string    `gorm:"type:varchar(500);"`
	UserInput     string    `gorm:"type:varchar(100);"`
	Data          string    `gorm:"index;type:varchar(500);"`
	Succeeded     bool      `gorm:"index;type:tinyint(1)"`
	StatusMessage string    `gorm:"type:varchar(500);"`
	CreatedAt     time.Time `gorm:"primaryKey;not null;type:datetime(6)"`
}

func (*SessionRequest) TableName() string {
	if sessionLogsTable != "" {
		return sessionLogsTable
	}
	return defaultSessionsLogsTable
}
