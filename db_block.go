package gitdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
)

//Block represents a block file
type Block struct {
	DataSet    *DataSet
	Name       string
	Size       int64
	Records    []*record
	BadRecords []string
}

//HumanSize returns human readable size of a block
func (b *Block) HumanSize() string {
	return formatBytes(uint64(b.Size))
}

//RecordCount returns the number of records in a block
func (b *Block) RecordCount() int {
	b.loadRecords()
	return len(b.Records)
}

//loadRecords loads all records in a block into memory
func (b *Block) loadRecords() {
	//only load record once per block
	if len(b.Records) == 0 {
		b.Records = b.records()
	}
}

func (b *Block) readBlock() ([]string, error) {

	var result []string

	blockFile := filepath.Join(b.DataSet.DbPath, b.DataSet.Name, b.Name+".json")
	log("Reading block: " + blockFile)
	data, err := ioutil.ReadFile(blockFile)
	if err != nil {
		return result, err
	}

	var dataBlock map[string]interface{}
	var record map[string]interface{}

	if err := json.Unmarshal(data, &dataBlock); err != nil {
		logError(err.Error())
		return result, &badBlockError{err.Error() + " - " + blockFile, blockFile}
	}

	recordKeys := orderMapKeys(dataBlock)

	//validates each record json and return a formatted version of the record
	for _, k := range recordKeys {
		//TODO handle encrypted records
		recordJSON := dataBlock[k].(string)
		if jsonErr := json.Unmarshal([]byte(recordJSON), &record); jsonErr != nil {
			return result, &badRecordError{jsonErr.Error() + " - " + k, k}
		}

		var buf bytes.Buffer
		if jsonErr := json.Indent(&buf, []byte(recordJSON), "", "\t"); jsonErr != nil {
			return result, &badRecordError{jsonErr.Error() + " - " + k, k}
		}

		result = append(result, buf.String())
	}

	return result, err
}

func (b *Block) records() []*record {

	var records []*record
	b.DataSet.BadBlocks = []string{}
	b.DataSet.BadRecords = []string{}

	recs, err := b.readBlock()

	if err != nil {
		if be, ok := err.(*badBlockError); ok {
			b.DataSet.BadBlocks = append(b.DataSet.BadBlocks, be.blockFile)
		} else if re, ok := err.(*badRecordError); ok {
			b.DataSet.BadRecords = append(b.DataSet.BadRecords, re.recordID)
		}

		return records
	}

	for _, rec := range recs {
		records = append(records, &record{Content: rec})
	}

	return records
}

//table returns a tabular representation of a Block
func (b *Block) table() *table {
	b.loadRecords()
	t := &table{}
	var jsonMap map[string]interface{}

	//TODO support backward compatibility
	var rawV2 struct {
		Indexes map[string]interface{}
		Data    map[string]interface{}
	}

	for i, record := range b.Records {
		if err := json.Unmarshal([]byte(record.Content), &rawV2); err != nil {
			logError(err.Error())
		}

		b, err := json.Marshal(rawV2.Data)
		if err != nil {
			logError(err.Error())
		}

		if err := json.Unmarshal(b, &jsonMap); err != nil {
			logError(err.Error())
		}

		var row []string
		if i == 0 {
			t.Headers = orderMapKeys(jsonMap)
		}
		for _, key := range t.Headers {
			val := fmt.Sprintf("%v", jsonMap[key])
			if len(val) > 40 {
				val = val[0:40]
			}
			row = append(row, val)
		}

		t.Rows = append(t.Rows, row)
	}

	return t
}

type gBlock struct {
	//recs used to provide hydration and sorting
	recs    map[string]*record
	dataset string
}

func newBlock(dataset string) *gBlock {
	block := &gBlock{dataset: dataset}
	block.recs = map[string]*record{}
	return block
}

func (b *gBlock) MarshalJSON() ([]byte, error) {
	raw := map[string]string{}
	for k, v := range b.recs {
		raw[k] = v.data
	}

	return json.Marshal(raw)
}

func (b *gBlock) UnmarshalJSON(data []byte) error {
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	//populate recs
	for k, v := range raw {
		b.recs[k] = newRecord(k, v)
	}

	return nil
}

func (b *gBlock) add(key string, value string) {
	b.recs[key] = newRecord(key, value)
}

func (b *gBlock) get(key string) (*record, error) {
	if _, ok := b.recs[key]; ok {
		return b.recs[key], nil
	}

	return nil, errors.New("key does not exist")
}

func (b *gBlock) delete(key string) error {
	if _, ok := b.recs[key]; ok {
		delete(b.recs, key)
		return nil
	}

	return errors.New("key does not exist")
}

func (b *gBlock) size() int {
	return len(b.recs)
}

func (b *gBlock) records(key string) []*record {
	var records []*record
	for _, v := range b.recs {
		v.key = key
		records = append(records, v)
	}

	sort.Sort(collection(records))

	return records
}

const (
	sizeByte = 1.0 << (10 * iota)
	sizeKb
	sizeMb
	sizeGb
	sizeTb
)

func formatBytes(bytes uint64) string {
	unit := ""
	value := float32(bytes)

	switch {
	case bytes >= sizeTb:
		unit = "TB"
		value = value / sizeTb
	case bytes >= sizeGb:
		unit = "GB"
		value = value / sizeGb
	case bytes >= sizeMb:
		unit = "MB"
		value = value / sizeMb
	case bytes >= sizeKb:
		unit = "KB"
		value = value / sizeKb
	case bytes >= sizeByte:
		unit = "B"
	case bytes == 0:
		return "0"
	}

	stringValue := fmt.Sprintf("%.1f", value)
	stringValue = strings.TrimSuffix(stringValue, ".0")
	return fmt.Sprintf("%s%s", stringValue, unit)
}

func orderMapKeys(_map map[string]interface{}) []string {
	// To store the keys in slice in sorted order
	var keys []string
	for k := range _map {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}