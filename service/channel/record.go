package channel

import (
	"fmt"

	"openapi/common"
	"openapi/model"
)

func ListTestRecords(filter model.ChannelTestRecordFilter, startIdx, pageSize int) ([]*model.ChannelTestRecord, int64, error) {
	return model.ListChannelTestRecords(filter, startIdx, pageSize)
}

func DeleteTestRecords(filter model.ChannelTestRecordFilter) (int64, error) {
	if filter.Empty() {
		return 0, validationErr("at least one filter condition is required")
	}
	return model.DeleteChannelTestRecords(filter)
}

func ClearTestRecords() (int64, error) {
	return model.ClearChannelTestRecords()
}

func retestRecord(record *model.ChannelTestRecord) (ChannelTestResult, error) {
	ch, err := model.GetChannelById(record.ChannelId, true)
	if err != nil {
		return ChannelTestResult{Model: record.ModelName}, fmt.Errorf("channel not found")
	}
	res := TestChannelRaw(ch, record.ModelName)
	status := model.ChannelTestStatusSuccess
	message := ""
	switch {
	case res.Skipped:
		status, message = model.ChannelTestStatusFail, "skipped: "+res.SkipReason
	case res.Err != nil:
		status, message = model.ChannelTestStatusFail, res.Err.MaskSensitiveErrorWithStatusCode()
	}
	if err := record.SaveTestResult(status, int(res.Latency.Milliseconds()), message, common.GetTimestamp()); err != nil {
		return res, err
	}
	record.Status = status
	record.ErrorMessage = message
	record.ResponseTime = int(res.Latency.Milliseconds())
	return res, nil
}

func RetestChannelModel(recordID int) (*model.ChannelTestRecord, error) {
	record, err := model.GetChannelTestRecordById(recordID)
	if err != nil {
		return nil, validationErr("test record not found")
	}
	if _, err := retestRecord(record); err != nil {
		return nil, validationErr(err.Error())
	}
	return record, nil
}

func BatchRetestChannelModels(ids []int) (total, passed, failed int) {
	for _, id := range ids {
		record, err := model.GetChannelTestRecordById(id)
		if err != nil {
			failed++
			continue
		}
		res, err := retestRecord(record)
		if err != nil || !res.OK() {
			failed++
			continue
		}
		passed++
	}
	return len(ids), passed, failed
}

func RetestFilteredRecords(filter model.ChannelTestRecordFilter) (total, passed, failed int, err error) {
	if filter.Empty() {
		return 0, 0, 0, validationErr("at least one filter condition is required")
	}
	records, err := model.FindChannelTestRecords(filter)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, record := range records {
		res, err := retestRecord(record)
		if err != nil || !res.OK() {
			failed++
			continue
		}
		passed++
	}
	return len(records), passed, failed, nil
}
