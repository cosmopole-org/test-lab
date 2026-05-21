package tools

import (
	"kasper/src/abstract/adapters/file"
	"kasper/src/abstract/adapters/network"
	"kasper/src/abstract/adapters/security"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	"kasper/src/abstract/adapters/vmm"
)

type ITools interface {
	Security() security.ISecurity
	Signaler() signaler.ISignaler
	Storage() storage.IStorage
	Network() network.INetwork
	File() file.IFile
	Vmm() vmm.IVmm
}
