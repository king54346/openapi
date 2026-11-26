package cron

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"openapi/model"
)

var ErrValidation = errors.New("validation error")

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrValidation, msg)
}

type CronJobInput struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Cron    string          `json:"cron"`
	Payload json.RawMessage `json:"payload"`
	Remark  string          `json:"remark"`
	Enabled bool            `json:"enabled"`
}

func (req *CronJobInput) Validate() error {
	if strings.TrimSpace(req.Name) == "" {
		return validationErr("name is required")
	}
	if _, ok := GetExecutor(req.Type); !ok {
		return validationErr("unsupported job type: " + req.Type)
	}
	if _, err := Parse(req.Cron); err != nil {
		return validationErr("invalid cron expression: " + err.Error())
	}
	if len(req.Payload) > 0 && !json.Valid(req.Payload) {
		return validationErr("payload must be valid JSON")
	}
	return nil
}

func (req *CronJobInput) ApplyTo(job *model.CronJob) {
	job.Name = strings.TrimSpace(req.Name)
	job.Type = req.Type
	job.Cron = strings.TrimSpace(req.Cron)
	job.Payload = string(req.Payload)
	job.Remark = req.Remark
	job.Enabled = req.Enabled
}

func ListCronJobs(startIdx, pageSize int) ([]*model.CronJob, int64, error) {
	return model.GetCronJobs(startIdx, pageSize)
}

func GetCronJob(id int64) (*model.CronJob, error) {
	return model.GetCronJobById(id)
}

func CreateCronJob(req CronJobInput) (*model.CronJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	job := &model.CronJob{}
	req.ApplyTo(job)
	if err := job.Insert(); err != nil {
		return nil, err
	}
	return job, nil
}

func UpdateCronJob(id int64, req CronJobInput) (*model.CronJob, error) {
	job, err := model.GetCronJobById(id)
	if err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	req.ApplyTo(job)
	if err := job.Update(); err != nil {
		return nil, err
	}
	return job, nil
}

func ToggleCronJob(id int64, enabled bool) error {
	return model.SetCronJobEnabled(id, enabled)
}

func DeleteCronJob(id int64) error {
	if _, err := model.GetCronJobById(id); err != nil {
		return err
	}
	return model.DeleteCronJobById(id)
}

func RunCronJobNow(id int64) error {
	job, err := model.GetCronJobById(id)
	if err != nil {
		return err
	}
	if _, ok := GetExecutor(job.Type); !ok {
		return validationErr("unsupported job type: " + job.Type)
	}
	if IsJobRunning(job.Id) {
		return validationErr("job is already running, check the run history later")
	}
	TriggerJobAsync(job)
	return nil
}

func GetCronJobRuns(id int64, startIdx, pageSize int) ([]*model.CronJobRun, int64, error) {
	return model.GetCronJobRuns(id, startIdx, pageSize)
}

func UnwrapBusinessErr(err error) (string, bool) {
	if errors.Is(err, ErrValidation) {
		return strings.TrimPrefix(err.Error(), ErrValidation.Error()+": "), true
	}
	if errors.Is(err, model.ErrCronJobNotFound) {
		return "cron job not found", true
	}
	return "", false
}
