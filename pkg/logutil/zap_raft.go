package logutil

import (
	"errors"

	"go.uber.org/zap"
)

// NewRaftLogger builds "raft.Logger" from "*zap.Config".
func NewRaftLogger(lcfg *zap.Config) (*zap.Logger, error) {
	if lcfg == nil {
		return nil, errors.New("nil zap.Config")
	}
	lg, err := lcfg.Build()
	if err != nil {
		return nil, err
	}
	return lg, nil
}
