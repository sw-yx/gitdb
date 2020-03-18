package gitdb

import (
	"os"
	"path/filepath"
	"time"
	"io/ioutil"
	"encoding/json"
)

type mail struct {
	Subject string
	Body    string
	Date    time.Time
}

type Mail struct {
	privateMail *mail
	dbConn      *Gitdb
}

// Implement json.Unmarshaller
func (m *Mail) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &m.privateMail)
}

func newMail(dbConn *Gitdb, subject string, body string) *Mail {
	return &Mail{
		privateMail: &mail{Subject: subject, Body: body, Date: time.Now()},
		dbConn:      dbConn,
	}
}

func (m *Mail) GetSubject() string {
	return m.privateMail.Subject
}

func (m *Mail) GetBody() string {
	return m.privateMail.Body
}

func (m *Mail) GetDate() time.Time {
	return m.privateMail.Date
}

func (m *Mail) send() error {

	if _, err := os.Stat(m.dbConn.mailDir()); err != nil {
		os.MkdirAll(m.dbConn.mailDir(), 0744)
	}

	bytes, err := json.Marshal(m.privateMail)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(m.dbConn.mailDir(), time.Now().Format("20060102150405")+".json"), bytes, 0744)
	if err != nil {
		log("Could not send notification - " + err.Error())
	}

	return err
}

func (g *Gitdb) GetMails() []*mail {

	var mails []*mail
	files, err := ioutil.ReadDir(g.mailDir())
	if err != nil {
		logError(err.Error())
		return mails
	}

	var fileName string
	for _, file := range files {
		fileName = filepath.Join(g.mailDir(), file.Name())
		if filepath.Ext(fileName) == ".json" {
			data, err := ioutil.ReadFile(fileName)
			if err != nil {
				logError(err.Error())
				continue
			}

			var mail *Mail
			fmtErr := json.Unmarshal(data, &mail)
			if fmtErr != nil {
				logError(err.Error())
				continue
			}

			mails = append(mails, mail.privateMail)
		}
	}

	return mails
}