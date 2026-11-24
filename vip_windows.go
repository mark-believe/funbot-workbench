package main

import "golang.org/x/sys/windows/registry"

func vipMachineID() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer key.Close()
	id, _, err := key.GetStringValue("MachineGuid")
	return id, err
}
