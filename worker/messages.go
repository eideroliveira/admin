package worker

import (
	"net/http"

	"github.com/goplaid/x/i18n"
	"github.com/goplaid/x/presets"
)

const I18nWorkerKey i18n.ModuleKey = "I18nWorkerKey"

type Messages struct {
	StatusNew                string
	StatusScheduled          string
	StatusRunning            string
	StatusCancelled          string
	StatusDone               string
	StatusException          string
	StatusKilled             string
	FilterTabAll             string
	FilterTabRunning         string
	FilterTabScheduled       string
	FilterTabDone            string
	FilterTabErrors          string
	ActionCancelJob          string
	ActionAbortJob           string
	ActionUpdateJob          string
	ActionRerunJob           string
	DetailTitleStatus        string
	DetailTitleLog           string
	NoticeJobCannotBeAborted string
	NoticeJobWontBeExecuted  string
}

var Messages_en_US = &Messages{
	StatusNew:                "New",
	StatusScheduled:          "Scheduled",
	StatusRunning:            "Running",
	StatusCancelled:          "Cancelled",
	StatusDone:               "Done",
	StatusException:          "Exception",
	StatusKilled:             "Killed",
	FilterTabAll:             "All Jobs",
	FilterTabRunning:         "Running",
	FilterTabScheduled:       "Scheduled",
	FilterTabDone:            "Done",
	FilterTabErrors:          "Errors",
	ActionCancelJob:          "Cancel Job",
	ActionAbortJob:           "Abort Job",
	ActionUpdateJob:          "Update Job",
	ActionRerunJob:           "Rerun Job",
	DetailTitleStatus:        "Status",
	DetailTitleLog:           "Log",
	NoticeJobCannotBeAborted: "This job cannot be aborted/canceled/updated due to its status change",
	NoticeJobWontBeExecuted:  "This job won't be executed due to code being deleted/modified",
}

var Messages_zh_CN = &Messages{
	StatusNew:                "新建",
	StatusScheduled:          "计划",
	StatusRunning:            "运行中",
	StatusCancelled:          "取消",
	StatusDone:               "完成",
	StatusException:          "错误",
	StatusKilled:             "中止",
	FilterTabAll:             "全部",
	FilterTabRunning:         "运行中",
	FilterTabScheduled:       "计划",
	FilterTabDone:            "完成",
	FilterTabErrors:          "错误",
	ActionCancelJob:          "取消任务",
	ActionAbortJob:           "中止任务",
	ActionUpdateJob:          "更新任务",
	ActionRerunJob:           "重跑任务",
	DetailTitleStatus:        "状态",
	DetailTitleLog:           "日志",
	NoticeJobCannotBeAborted: "任务状态已经改变，不能被中止/取消/更新",
	NoticeJobWontBeExecuted:  "任务代码被删除/修改, 这个任务不会被执行",
}

func getTStatus(msgr *Messages, status string) string {
	switch status {
	case JobStatusNew:
		return msgr.StatusNew
	case JobStatusScheduled:
		return msgr.StatusScheduled
	case JobStatusRunning:
		return msgr.StatusRunning
	case JobStatusCancelled:
		return msgr.StatusCancelled
	case JobStatusDone:
		return msgr.StatusDone
	case JobStatusException:
		return msgr.StatusException
	case JobStatusKilled:
		return msgr.StatusKilled
	}
	return status
}

func getTJob(r *http.Request, v string) string {
	return i18n.PT(r, presets.ModelsI18nModuleKey, "WorkerJob", v)
}
