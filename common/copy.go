package common

import (
	"fmt"

	"github.com/jinzhu/copier"
)

func DeepCopy[T any](src *T) (*T, error) {
	if src == nil {
		return nil, fmt.Errorf("copy source cannot be nil")
	}
	var dst T
	// NOTE: do not use IgnoreEmpty here, otherwise zero values in src
	// would be silently dropped and the copy would not be faithful.
	err := copier.CopyWithOption(&dst, src, copier.Option{DeepCopy: true})
	if err != nil {
		return nil, err
	}
	return &dst, nil
}
