package errors

import "errors"

var (
	//internal errors
	errDb                = errors.New("gitDB: database error")
	errBadBlock          = errors.New("gitDB: Bad block error - invalid json")
	errBadRecord         = errors.New("gitDB: Bad record error")
	errConnectionClosed  = errors.New("gitDB: connection is closed")
	errConnectionInvalid = errors.New("gitDB: connection is not valid. use gitdb.Start to construct a valid connection")

	//external errors
	ErrNoRecords       = errors.New("gitDB: no records found")
	ErrRecordNotFound  = errors.New("gitDB: record not found")
	ErrInvalidRecordID = errors.New("gitDB: invalid record id")
)