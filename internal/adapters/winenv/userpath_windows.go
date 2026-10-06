// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build windows

package winenv

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/kivoradigital/wspace/internal/ports"
)

// Store reads and writes HKCU\Environment\Path through the registry API,
// so values of any length are kept whole (setx truncates at 1024
// characters) and %VAR% references stay unexpanded.
type Store struct{}

var _ ports.UserPathStore = Store{}

// New returns the registry-backed store.
func New() Store { return Store{} }

const (
	envKey   = `Environment`
	pathName = "Path"
)

func (Store) UserPath() (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, envKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	defer func() { _ = k.Close() }()
	// GetStringValue returns REG_EXPAND_SZ data unexpanded.
	val, typ, err := k.GetStringValue(pathName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, typ == registry.EXPAND_SZ, nil
}

func (Store) SetUserPath(value string, expand bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, envKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if expand {
		return k.SetExpandStringValue(pathName, value)
	}
	return k.SetStringValue(pathName, value)
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
)

const (
	hwndBroadcast   = 0xffff
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

// BroadcastEnvironmentChange sends WM_SETTINGCHANGE("Environment") to
// every top-level window, as the System Properties dialog does, so
// Explorer reloads the environment for the programs it starts next. A
// hung window is skipped after 5 seconds.
func (Store) BroadcastEnvironmentChange() error {
	param, err := windows.UTF16PtrFromString(envKey)
	if err != nil {
		return err
	}
	var result uintptr
	r, _, callErr := procSendMessageTimeoutW.Call(
		hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(param)),
		smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
	if r == 0 {
		return callErr
	}
	return nil
}
