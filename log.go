package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type contextKey string

const fileDelim = ""

const (
	ReqContextRequestID contextKey = "request_id"
)

func Println(ctx context.Context, logs ...interface{}) {
	requestID := "<empty>"
	if ctx != nil && ctx.Value(ReqContextRequestID) != nil {
		requestID = ctx.Value(ReqContextRequestID).(string)
	}

	logsIntf := []interface{}{"[RequestID: ", requestID, " ]"}
	logsIntf = append(logsIntf, logs...)
	PlainPrintln(logsIntf...)
}

func Printf(ctx context.Context, format string, logs ...interface{}) {
	requestID := "<empty>"
	if ctx != nil && ctx.Value(ReqContextRequestID) != nil {
		requestID = ctx.Value(ReqContextRequestID).(string)
	}

	logsIntf := []interface{}{requestID}
	logsIntf = append(logsIntf, logs...)
	PlainPrintf("[RequestID: %s ] "+format, logsIntf...)
}

func DetailedLogRequestTimestamp(ctx context.Context, shouldPrint bool, currentTime *time.Time, title string, message ...string) string {
	logTime := time.Now()

	text := ""
	if currentTime != nil {
		text = fmt.Sprint(title, "-", message, " Log time span ", " ,Time: ", logTime, " ,Diff: ", logTime.Sub(*currentTime))
		*currentTime = logTime
	}

	if !shouldPrint {
		return text
	}

	Println(ctx, text)
	return text
}

func extractLogDetails(path string, linenum int, ok bool) (string, int) {
	file := "<???>"
	line := 1

	if ok {
		slash := strings.LastIndex(path, fileDelim)
		line = linenum
		file = path[slash+len(fileDelim):]
	}

	return file, line
}

func Error(args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)
	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Error(args...)
}

func Fatalln(args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)

	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Fatalln(args...)
}

func Debugf(format string, args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)

	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Debugf(format, args...)
}

func Info(args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)
	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Info(args...)
}

func Infof(format string, args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)
	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Infof(format, args...)
}

func Infoln(args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)
	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Infoln(args...)
}

func PlainPrintln(args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)
	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Infoln(args...)
}

func PlainPrintf(format string, args ...interface{}) {
	_, file, line, ok := runtime.Caller(2)
	file, line = extractLogDetails(file, line, ok)
	logrus.WithField("source", fmt.Sprintf("%s:%d", file, line)).Infof(format, args...)
}
