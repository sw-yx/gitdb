package gitdb

import (
	"time"
)

type DataFormat string

const (
	JSON DataFormat = "json"
	BSON DataFormat = "bson"
	CSV  DataFormat = "csv"
)

type Model interface {
	Id() string
	SetId(string)
	SetMetaData(m *metaData)
	String() string
	Validate() bool
	IsLockable() bool
	GetLockFileNames() []string
	GetSchema() *Schema
	GetCreatedDate() time.Time
	GetUpdatedDate() time.Time
	SetCreatedDate(time.Time)
	SetUpdatedDate(time.Time)
	ShouldEncrypt() bool
	GetValidationErrors() []error
}

type metaData struct {
	Indexes   map[string]interface{}
	Encrypted bool
}

type BaseModel struct {
	ID        string
	Meta      *metaData
	CreatedAt time.Time
	UpdatedAt time.Time
	errors    []error
}

func (m *BaseModel) Id() string {
	return m.ID
}

func (m *BaseModel) SetId(id string) {
	m.ID = id
}

func (m *BaseModel) SetMetaData(md *metaData) {
	m.Meta = md
}

func (m *BaseModel) GetCreatedDate() time.Time {
	return m.CreatedAt
}

func (m *BaseModel) GetUpdatedDate() time.Time {
	return m.UpdatedAt
}

func (m *BaseModel) SetCreatedDate(t time.Time) {
	m.CreatedAt = t
}

func (m *BaseModel) SetUpdatedDate(t time.Time) {
	m.UpdatedAt = t
}

func (m *BaseModel) String() string {
	return m.ID
}

func (m *BaseModel) Validate() bool {
	return true
}

func (m *BaseModel) IsLockable() bool {
	return false
}

func (m *BaseModel) GetLockFileNames() []string {
	return []string{}
}

func (m *BaseModel) ShouldEncrypt() bool {
	return false
}

func (m *BaseModel) GetValidationErrors() []error {
	return m.errors
}
