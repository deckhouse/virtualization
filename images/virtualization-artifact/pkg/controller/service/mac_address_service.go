/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
)

const MaxCount int = 16777216

type MACAddressService struct {
	oui string
}

func NewMACAddressService(
	oui string,
) *MACAddressService {
	return &MACAddressService{
		oui: oui,
	}
}

func (s MACAddressService) IsAvailableAddress(address string, allocatedMACs mac.AllocatedMACs) error {
	if !mac.IsValidAddressFormat(address) {
		return errors.New("invalid MAC address format")
	}

	if _, ok := allocatedMACs[address]; ok {
		// already exists
		return ErrMACAddressAlreadyExist
	}

	if address[:8] == s.oui {
		return nil
	}

	return ErrMACAddressOutOfRange
}

func formatOUI(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)

	re := regexp.MustCompile(`(?i)([0-9A-Fa-f]{2})`)
	matches := re.FindAllString(prefix, -1)

	if len(matches) != 3 {
		return "", fmt.Errorf("wrong format MAC address oui")
	}

	return fmt.Sprintf("%s:%s:%s", matches[0], matches[1], matches[2]), nil
}

func (s MACAddressService) AllocateNewAddress(allocatedMACs mac.AllocatedMACs) (string, error) {
	prefix, err := formatOUI(s.oui)
	if err != nil {
		return "", err
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	retry := 0
	maxRetries := MaxCount - len(allocatedMACs)

	for retry < maxRetries {
		genAddress := fmt.Sprintf("%s:%02x:%02x:%02x", prefix, r.Intn(256), r.Intn(256), r.Intn(256))
		if _, ok := allocatedMACs[genAddress]; !ok {
			return genAddress, nil
		}
		retry++
	}

	return "", errors.New("no remaining MAC addresses")
}
