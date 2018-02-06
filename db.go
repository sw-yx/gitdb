package db

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	//"vogue/log"
	"gopkg.in/mgo.v2/bson"
)

type SearchMode string

const (
	SEARCH_MODE_EQUALS SearchMode = "equals"
	SEARCH_MODE_CONTAINS SearchMode = "contains"
	SEARCH_MODE_STARTS_WITH SearchMode = "starts_with"
	SEARCH_MODE_ENDS_WITH SearchMode = "ends_with"
)

type SearchQuery struct {
	DataDir string
	Indexes []string
	Values  []string
	Mode    SearchMode
}

func Insert(m ModelSchema) error {

	if m.GetCreatedDate().IsZero() {
		m.stampCreatedDate()
	}
	m.stampUpdatedDate()

	if !m.Validate() {
		return errors.New("Model is not valid")
	}

	m.SetId(m.GetID().RecordId())
	newRecordBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	fullPath = filepath.Join(dbPath, m.Name())

	if _, err := os.Stat(fullPath); err != nil {
		os.MkdirAll(fullPath, 0755)
	}


	dataBlock := map[string]string{}
	recordExists := false

	dataFileName := m.GetID().blockId() + "." + string(m.GetDataFormat())
	dataFilePath := filepath.Join(fullPath, dataFileName)
	commitMsg := "Inserting " + m.GetID().RecordId() + " into " + dataFileName
	//log.PutInfo(commitMsg)
	events <- newWriteBeforeEvent("...", dataFileName)
	if _, err := os.Stat(dataFilePath); err == nil {
		//block file exist, read it, check for duplicates and append new data
		records, err := readBlock(dataFilePath, m)
		if err != nil {
			return err
		}

		for _, record := range records {
			println(record.GetID().RecordId())
			if record.GetID().RecordId() == m.GetID().RecordId() {
				recordExists = true
				//overwrite existing record
				//log.PutInfo("Overwriting record - " + m.Id())

				dataBlock[record.GetID().RecordId()] = string(newRecordBytes)

			} else {
				recordBytes, err := json.Marshal(record)
				if err != nil {
					return err
				}

				dataBlock[record.GetID().RecordId()] = string(recordBytes)
			}
		}
	}

	if !recordExists {
		dataBlock[m.GetID().RecordId()] = string(newRecordBytes)
	}

	var blockBytes []byte
	var fmtErr error
	switch m.GetDataFormat() {
	case JSON:
		blockBytes, fmtErr = json.MarshalIndent(dataBlock, "", "\t")
		break
	case BSON:
		blockBytes, fmtErr = bson.Marshal(dataBlock)
		break
	}

	if fmtErr != nil {
		return fmtErr
	}

	writeErr := ioutil.WriteFile(dataFilePath, blockBytes, 0744)
	if writeErr == nil {
		events <- newWriteEvent(commitMsg, dataFileName)
	}else{
		return writeErr
	}

	defer updateIndexes(m)

	return err
}

func readBlock(blockFile string, m ModelSchema) ([]ModelSchema, error) {

	var result []ModelSchema
	var jsonErr error

	data, err := ioutil.ReadFile(blockFile)
	if err != nil {
		//log.PutError(err.Error())
		return result, err
	}

	dataBlock := map[string]string{}
	var fmtErr error
	switch m.GetDataFormat() {
	case JSON:
		fmtErr = json.Unmarshal(data, &dataBlock)
		break
	case BSON:
		fmtErr = bson.Unmarshal(data, &dataBlock)
		break
	}

	if fmtErr != nil {
		//log.PutError(fmtErr.Error())
		return result, fmtErr
	}

	for _, v := range dataBlock {

		concreteModel := factory(m.Name())

		jsonErr = json.Unmarshal([]byte(v), concreteModel)
		if jsonErr != nil {
			//log.PutError(jsonErr.Error())
			return result, jsonErr
		}

		result = append(result, concreteModel.(ModelSchema))
	}

	return result, err
}

func parseId(id string) (dataDir string, block string, record string, err error) {
	recordMeta := strings.Split(id, "/")
	if len(recordMeta) != 3 {
		err = errors.New("Invalid record id")
	} else {
		dataDir = recordMeta[0]
		block = recordMeta[1]
		record = recordMeta[2]
	}

	return dataDir, block, record, err
}

func Get(id string) (ModelSchema, error) {

	var m ModelSchema

	dataDir, block, _, err := parseId(id)
	if err != nil {
		return m, err
	}

	dataFilePath := filepath.Join(dbPath, dataDir, block+"."+string(m.GetDataFormat()))
	if _, err := os.Stat(dataFilePath); err != nil {
		return m, errors.New(dataDir + " Not Found - " + id)
	} else {
		model := factory(dataDir)
		records, err := readBlock(dataFilePath, model)
		if err != nil {
			return m, err
		}

		for _, record := range records {
			if record.GetID().RecordId() == id {
				return record, nil
			}
		}
	}

	events <- newReadEvent("...", id)
	return m, errors.New("Record "+id+" not found in " + dataDir)
}

func Fetch(dataDir string) ([]ModelSchema, error) {

	var records []ModelSchema

	fullPath := filepath.Join(dbPath, dataDir)
	events <- newReadEvent("...", fullPath)
	//log.PutInfo("Fetching records from - " + fullPath)
	files, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return records, err
	}

	model := factory(dataDir)
	for _, file := range files {
		fileName := filepath.Join(fullPath, file.Name())
		if filepath.Ext(fileName) == "."+string(model.GetDataFormat()) {
			results, err := readBlock(fileName, model)
			if err != nil {
				return records, nil
			}
			records = append(records, results...)
		}
	}

	//log.PutInfo(fmt.Sprintf("%d records found in %s", len(records), fullPath))
	return records, nil
}

func Search(dataDir string, searchIndexes []string, searchValues []string, searchMode SearchMode) ([]ModelSchema, error) {

	query := &SearchQuery{
		DataDir: dataDir,
		Indexes:   searchIndexes,
		Values:  searchValues,
		Mode:    searchMode,
	}

	var records []ModelSchema
	matchingRecords := make(map[string]string)
	//log.PutInfo(fmt.Sprintf("Searching "+query.DataDir+" namespace by %s for '%s'", query.Index, strings.Join(query.Values, ",")))
	for _, index := range query.Indexes {
		indexFile := filepath.Join(indexDir(), query.DataDir, index+".json")
		if _, err := os.Stat(indexFile); err != nil {
			return records, errors.New(index+" index does not exist")
		}

		events <- newReadEvent("...", indexFile)

		index := readIndex(indexFile)

		for k, v := range index {
			addResult := false
			dbValue := strings.ToLower(v.(string))
			for _, value := range query.Values {
				queryValue := strings.ToLower(value)
				switch query.Mode {
				case SEARCH_MODE_EQUALS:
					addResult = (dbValue == queryValue)
					break
				case SEARCH_MODE_CONTAINS:
					addResult = strings.Contains(dbValue, queryValue)
					break
				case SEARCH_MODE_STARTS_WITH:
					addResult = strings.HasPrefix(dbValue, queryValue)
					break
				case SEARCH_MODE_ENDS_WITH:
					addResult = strings.HasSuffix(dbValue, queryValue)
					break
				}

				if addResult {
					matchingRecords[k] = v.(string)
				} else if _, ok := matchingRecords[k]; ok {
					delete(matchingRecords, k)
				}
			}
		}
	}

	//filter out the blocks that we need to search
	searchBlocks := map[string]string{}
	for recordId := range matchingRecords {
		_, block, _, err := parseId(recordId)
		if err != nil {
			return records, err
		}

		searchBlocks[block] = block
	}

	for _, block := range searchBlocks {

		model := factory(query.DataDir)

		blockFile := OsPath(filepath.Join(dbPath, query.DataDir, block+"."+ string(model.GetDataFormat())))
		blockRecords, err := readBlock(blockFile, model)
		if err != nil {
			return records, err
		}

		for _, record := range blockRecords {
			if _, ok := matchingRecords[record.GetID().RecordId()]; ok {
				records = append(records, record)
			}
		}
	}

	//log.PutInfo(fmt.Sprintf("Found %d results in %s namespace by %s for '%s'", len(records), query.DataDir, query.Index, strings.Join(query.Values, ",")))
	return records, nil
}

func Delete(id string) (bool, error) {
	return del(id, false)
}

func DeleteOrFail(id string) (bool, error) {
	return del(id, true)
}

func del(id string, failIfNotFound bool) (bool, error) {

	dataDir, block, _, err := parseId(id)
	if err != nil {
		return false, err
	}

	model := factory(dataDir)

	dataFileName := filepath.Join(dbPath, dataDir, block+"."+string(model.GetDataFormat()))
	if _, err := os.Stat(dataFileName); err != nil {
		if failIfNotFound {
			return false, errors.New("Could not delete [" + id + "]: record does not exist")
		}
		return true, nil
	}

	records, err := readBlock(dataFileName, model)
	if err != nil {
		return false, err
	}

	deleteRecordFound := false
	blockData := map[string]string{}
	for _, record := range records {
		if record.GetID().RecordId() != id {
			data, err := json.Marshal(record)
			if err != nil {
				return false, err
			}

			blockData[record.GetID().RecordId()] = string(data)
		} else {
			deleteRecordFound = true
		}
	}

	if deleteRecordFound {

		out, err := json.MarshalIndent(blockData, "", "\t")
		if err != nil {
			return false, err
		}

		//write undeleted records back to block file
		err = ioutil.WriteFile(dataFileName, out, 0744)
		if err != nil {
			return false, err
		}
		return true, nil
	} else {
		if failIfNotFound {
			return false, errors.New("Could not delete [" + id + "]: record does not exist")
		}

		return true, nil
	}
}

func OsPath(path string) string {
	if runtime.GOOS == "windows" {
		return strings.Replace(path, "/", string(filepath.Separator), -1)
	}
	return strings.Replace(path, "\\", string(filepath.Separator), -1)
}

func RandStr(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
