package server2

import (
	"github.com/onflow/cadence/common"
	"github.com/onflow/cadence/errors"
	"github.com/onflow/cadence/interpreter"
	"github.com/onflow/cadence/sema"
	"github.com/onflow/cadence/stdlib"
)

// stdlibHandler implements stdlib.StandardLibraryHandler for type-checking only.
// All runtime methods panic because they should never be called during analysis.
type stdlibHandler struct{}

var _ stdlib.StandardLibraryHandler = stdlibHandler{}

func (stdlibHandler) ProgramLog(_ string) error {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) UnsafeRandom() (uint64, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetBlockAtHeight(_ uint64) (stdlib.Block, bool, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetCurrentBlockHeight() (uint64, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetAccountBalance(_ common.Address) (uint64, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetAccountAvailableBalance(_ common.Address) (uint64, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) CommitStorageTemporarily(_ interpreter.ValueTransferContext) error {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetStorageUsed(_ common.Address) (uint64, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetStorageCapacity(_ common.Address) (uint64, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetAccountKey(_ common.Address, _ uint32) (*stdlib.AccountKey, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetAccountContractNames(_ common.Address) ([]string, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GetAccountContractCode(_ common.AddressLocation) ([]byte, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) EmitEvent(
	_ interpreter.ValueExportContext,
	_ *sema.CompositeType,
	_ []interpreter.Value,
) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) AddAccountKey(
	_ common.Address,
	_ *stdlib.PublicKey,
	_ sema.HashAlgorithm,
	_ int,
) (*stdlib.AccountKey, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) RevokeAccountKey(_ common.Address, _ uint32) (*stdlib.AccountKey, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) ParseAndCheckProgram(_ []byte, _ common.Location, _ bool) (*interpreter.Program, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) UpdateAccountContractCode(_ common.AddressLocation, _ []byte) error {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) RecordContractUpdate(_ common.AddressLocation, _ *interpreter.CompositeValue) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) ContractUpdateRecorded(_ common.AddressLocation) bool {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) InterpretContract(
	_ common.AddressLocation,
	_ *interpreter.Program,
	_ string,
	_ stdlib.DeployedContractConstructorInvocation,
) (*interpreter.CompositeValue, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) TemporarilyRecordCode(_ common.AddressLocation, _ []byte) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) RemoveAccountContractCode(_ common.AddressLocation) error {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) RecordContractRemoval(_ common.AddressLocation) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) CreateAccount(_ common.Address) (common.Address, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) ValidatePublicKey(_ *stdlib.PublicKey) error {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) VerifySignature(
	_ []byte,
	_ string,
	_ []byte,
	_ []byte,
	_ sema.SignatureAlgorithm,
	_ sema.HashAlgorithm,
) (bool, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) BLSVerifyPOP(_ *stdlib.PublicKey, _ []byte) (bool, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) Hash(_ []byte, _ string, _ sema.HashAlgorithm) ([]byte, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) AccountKeysCount(_ common.Address) (uint32, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) BLSAggregatePublicKeys(_ []*stdlib.PublicKey) (*stdlib.PublicKey, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) BLSAggregateSignatures(_ [][]byte) ([]byte, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) GenerateAccountID(_ common.Address) (uint64, error) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) ReadRandom(_ []byte) error {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) StartContractAddition(_ common.AddressLocation) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) EndContractAddition(_ common.AddressLocation) {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) IsContractBeingAdded(_ common.AddressLocation) bool {
	panic(errors.NewUnreachableError())
}

func (stdlibHandler) LoadContractValue(
	_ common.AddressLocation,
	_ *interpreter.Program,
	_ string,
	_ stdlib.DeployedContractConstructorInvocation,
) (*interpreter.CompositeValue, error) {
	panic(errors.NewUnreachableError())
}

// newBaseValueActivation builds a base value activation with the script standard library.
func newBaseValueActivation() *sema.VariableActivation {
	activation := sema.NewVariableActivation(sema.BaseValueActivation)
	handler := stdlibHandler{}
	for _, decl := range stdlib.InterpreterDefaultScriptStandardLibraryValues(handler) {
		activation.DeclareValue(decl)
	}
	return activation
}
