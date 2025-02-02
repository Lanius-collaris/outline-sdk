// Copyright 2023 Jigsaw Operations LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/transport/shadowsocks"
)

func newShadowsocksStreamDialerFromURL(innerDialer transport.StreamDialer, configURL *url.URL) (transport.StreamDialer, error) {
	config, err := parseShadowsocksURL(configURL)
	if err != nil {
		return nil, err
	}
	endpoint := &transport.StreamDialerEndpoint{Dialer: innerDialer, Address: config.serverAddress}
	dialer, err := shadowsocks.NewStreamDialer(endpoint, config.cryptoKey)
	if err != nil {
		return nil, err
	}
	if len(config.prefix) > 0 {
		dialer.SaltGenerator = shadowsocks.NewPrefixSaltGenerator(config.prefix)
	}
	return dialer, nil
}

func newShadowsocksPacketDialerFromURL(innerDialer transport.PacketDialer, configURL *url.URL) (transport.PacketDialer, error) {
	config, err := parseShadowsocksURL(configURL)
	if err != nil {
		return nil, err
	}
	endpoint := &transport.PacketDialerEndpoint{Dialer: innerDialer, Address: config.serverAddress}
	listener, err := shadowsocks.NewPacketListener(endpoint, config.cryptoKey)
	if err != nil {
		return nil, err
	}
	dialer := transport.PacketListenerDialer{Listener: listener}
	return dialer, nil
}

type shadowsocksConfig struct {
	serverAddress string
	cryptoKey     *shadowsocks.EncryptionKey
	prefix        []byte
}

func parseShadowsocksURL(url *url.URL) (*shadowsocksConfig, error) {
	config := &shadowsocksConfig{}
	if url.Host == "" {
		return nil, errors.New("host not specified")
	}
	config.serverAddress = url.Host
	cipherInfoBytes, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(url.User.String())
	if err != nil {
		return nil, fmt.Errorf("failed to decode cipher info [%v]: %w", url.User.String(), err)
	}
	cipherName, secret, found := strings.Cut(string(cipherInfoBytes), ":")
	if !found {
		return nil, errors.New("invalid cipher info: no ':' separator")
	}
	config.cryptoKey, err = shadowsocks.NewEncryptionKey(cipherName, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	prefixStr := url.Query().Get("prefix")
	if len(prefixStr) > 0 {
		config.prefix, err = parseStringPrefix(prefixStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse prefix: %w", err)
		}
	}
	return config, nil
}

func parseStringPrefix(utf8Str string) ([]byte, error) {
	runes := []rune(utf8Str)
	rawBytes := make([]byte, len(runes))
	for i, r := range runes {
		if (r & 0xFF) != r {
			return nil, fmt.Errorf("character out of range: %d", r)
		}
		rawBytes[i] = byte(r)
	}
	return rawBytes, nil
}
