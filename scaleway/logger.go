package scaleway

import (
	"fmt"

	"github.com/scaleway/scaleway-sdk-go/logger"
	"k8s.io/klog/v2"
)

// Logger completes klog to be able to implement the Logger interface for the SDK
type Logger struct{}

var logging Logger

// Debugf logs to DEBUG log. Arguments are handled in the manner of fmt.Printf.
func (Logger) Debugf(format string, args ...interface{}) {
	if klog.V(4).Enabled() {
		klog.InfoDepth(2, fmt.Sprintf(format, args...))
	}
}

// Infof logs to INFO log. Arguments are handled in the manner of fmt.Printf.
func (Logger) Infof(format string, args ...interface{}) {
	if klog.V(2).Enabled() {
		klog.InfoDepth(2, fmt.Sprintf(format, args...))
	}
}

// Warningf logs to WARNING log. Arguments are handled in the manner of fmt.Printf.
func (Logger) Warningf(format string, args ...interface{}) {
	klog.WarningDepth(2, fmt.Sprintf(format, args...))
}

// Errorf logs to ERROR log. Arguments are handled in the manner of fmt.Printf.
func (Logger) Errorf(format string, args ...interface{}) {
	klog.ErrorDepth(2, fmt.Sprintf(format, args...))
}

// ShouldLog reports whether verbosity level l is at least the requested verbose level.
func (Logger) ShouldLog(level logger.LogLevel) bool {
	switch level {
	case logger.LogLevelError:
		return true
	case logger.LogLevelWarning:
		return true
	case logger.LogLevelInfo:
		if klog.V(2).Enabled() {
			return true
		}
	case logger.LogLevelDebug:
		if klog.V(5).Enabled() {
			return true
		}
	default:
		return true
	}

	return false
}
