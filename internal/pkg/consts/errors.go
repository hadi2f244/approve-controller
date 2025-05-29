package consts

import "time"

type RequeueError struct{}

func (m *RequeueError) Error() string {
	return "Requeue"
}
func IsRequeueError(err error) bool {
	if _, ok := err.(*RequeueError); ok {
		return true
	} else {
		return false
	}
}

type RequeueAfterError struct {
	RequeueAfter time.Duration
}

func (m *RequeueAfterError) Error() string {
	return "Requeue"
}
func IsRequeueAfterError(err error) bool {
	if _, ok := err.(*RequeueAfterError); ok {
		return true
	} else {
		return false
	}
}

type LookupIPError struct{}

func (m *LookupIPError) Error() string {
	return "Requeue"
}
func IsLookupIPError(err error) bool {
	if _, ok := err.(*LookupIPError); ok {
		return true
	} else {
		return false
	}
}
