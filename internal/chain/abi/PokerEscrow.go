// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package abi

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// PokerEscrowMetaData contains all meta data concerning the PokerEscrow contract.
var PokerEscrowMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_tableID\",\"type\":\"string\"},{\"internalType\":\"uint8\",\"name\":\"_maxSeats\",\"type\":\"uint8\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockNumber\",\"type\":\"uint256\"}],\"name\":\"Abandoned\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"filer\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"accused\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"reason\",\"type\":\"string\"}],\"name\":\"DisputeFiled\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockNumber\",\"type\":\"uint256\"}],\"name\":\"GameStarted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"stateRoot\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockNumber\",\"type\":\"uint256\"}],\"name\":\"OutcomeReported\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"player\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"PayoutSent\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"player\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"peerID\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint8\",\"name\":\"seat\",\"type\":\"uint8\"}],\"name\":\"PlayerJoined\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"player\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"Refunded\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"player\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"slashedAmount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"burnedAmount\",\"type\":\"uint256\"}],\"name\":\"SlashExecuted\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"CHALLENGE_WINDOW\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"SETTLEMENT_DEADLINE\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"SIG_THRESHOLD_DEN\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"SIG_THRESHOLD_NUM\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"SLASH_BURN_BPS\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"gameStartBlock\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"peerID\",\"type\":\"string\"}],\"name\":\"joinTable\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"markAbandoned\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"maxSeats\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"seat\",\"type\":\"uint8\"}],\"name\":\"playerAt\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"peerID\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"buyIn\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"withdrawn\",\"type\":\"bool\"},{\"internalType\":\"bool\",\"name\":\"slashed\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"playerCount\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"players\",\"outputs\":[{\"internalType\":\"addresspayable\",\"name\":\"addr\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"peerID\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"buyIn\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"withdrawn\",\"type\":\"bool\"},{\"internalType\":\"bool\",\"name\":\"slashed\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"refund\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"int256[]\",\"name\":\"payoutDeltas\",\"type\":\"int256[]\"},{\"internalType\":\"bytes32\",\"name\":\"_stateRoot\",\"type\":\"bytes32\"},{\"internalType\":\"bytes[]\",\"name\":\"signatures\",\"type\":\"bytes[]\"},{\"internalType\":\"uint256\",\"name\":\"handNum\",\"type\":\"uint256\"}],\"name\":\"reportOutcome\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"requiredSignatures\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"seatOf\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"settlementBlock\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"slashEvidence\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"state\",\"outputs\":[{\"internalType\":\"enumPokerEscrow.TableState\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"stateRoot\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"accused\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"reason\",\"type\":\"string\"},{\"internalType\":\"bytes\",\"name\":\"evidence\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"accuserSig\",\"type\":\"bytes\"}],\"name\":\"submitDisput\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"tableID\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalEscrow\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"stateMutability\":\"payable\",\"type\":\"receive\"}]",
}

// PokerEscrowABI is the input ABI used to generate the binding from.
// Deprecated: Use PokerEscrowMetaData.ABI instead.
var PokerEscrowABI = PokerEscrowMetaData.ABI

// PokerEscrow is an auto generated Go binding around an Ethereum contract.
type PokerEscrow struct {
	PokerEscrowCaller     // Read-only binding to the contract
	PokerEscrowTransactor // Write-only binding to the contract
	PokerEscrowFilterer   // Log filterer for contract events
}

// PokerEscrowCaller is an auto generated read-only Go binding around an Ethereum contract.
type PokerEscrowCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PokerEscrowTransactor is an auto generated write-only Go binding around an Ethereum contract.
type PokerEscrowTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PokerEscrowFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type PokerEscrowFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PokerEscrowSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type PokerEscrowSession struct {
	Contract     *PokerEscrow      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// PokerEscrowCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type PokerEscrowCallerSession struct {
	Contract *PokerEscrowCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// PokerEscrowTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type PokerEscrowTransactorSession struct {
	Contract     *PokerEscrowTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// PokerEscrowRaw is an auto generated low-level Go binding around an Ethereum contract.
type PokerEscrowRaw struct {
	Contract *PokerEscrow // Generic contract binding to access the raw methods on
}

// PokerEscrowCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type PokerEscrowCallerRaw struct {
	Contract *PokerEscrowCaller // Generic read-only contract binding to access the raw methods on
}

// PokerEscrowTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type PokerEscrowTransactorRaw struct {
	Contract *PokerEscrowTransactor // Generic write-only contract binding to access the raw methods on
}

// NewPokerEscrow creates a new instance of PokerEscrow, bound to a specific deployed contract.
func NewPokerEscrow(address common.Address, backend bind.ContractBackend) (*PokerEscrow, error) {
	contract, err := bindPokerEscrow(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &PokerEscrow{PokerEscrowCaller: PokerEscrowCaller{contract: contract}, PokerEscrowTransactor: PokerEscrowTransactor{contract: contract}, PokerEscrowFilterer: PokerEscrowFilterer{contract: contract}}, nil
}

// NewPokerEscrowCaller creates a new read-only instance of PokerEscrow, bound to a specific deployed contract.
func NewPokerEscrowCaller(address common.Address, caller bind.ContractCaller) (*PokerEscrowCaller, error) {
	contract, err := bindPokerEscrow(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowCaller{contract: contract}, nil
}

// NewPokerEscrowTransactor creates a new write-only instance of PokerEscrow, bound to a specific deployed contract.
func NewPokerEscrowTransactor(address common.Address, transactor bind.ContractTransactor) (*PokerEscrowTransactor, error) {
	contract, err := bindPokerEscrow(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowTransactor{contract: contract}, nil
}

// NewPokerEscrowFilterer creates a new log filterer instance of PokerEscrow, bound to a specific deployed contract.
func NewPokerEscrowFilterer(address common.Address, filterer bind.ContractFilterer) (*PokerEscrowFilterer, error) {
	contract, err := bindPokerEscrow(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowFilterer{contract: contract}, nil
}

// bindPokerEscrow binds a generic wrapper to an already deployed contract.
func bindPokerEscrow(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := PokerEscrowMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_PokerEscrow *PokerEscrowRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _PokerEscrow.Contract.PokerEscrowCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_PokerEscrow *PokerEscrowRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PokerEscrow.Contract.PokerEscrowTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_PokerEscrow *PokerEscrowRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _PokerEscrow.Contract.PokerEscrowTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_PokerEscrow *PokerEscrowCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _PokerEscrow.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_PokerEscrow *PokerEscrowTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PokerEscrow.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_PokerEscrow *PokerEscrowTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _PokerEscrow.Contract.contract.Transact(opts, method, params...)
}

// CHALLENGEWINDOW is a free data retrieval call binding the contract method 0xd62aad29.
//
// Solidity: function CHALLENGE_WINDOW() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) CHALLENGEWINDOW(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "CHALLENGE_WINDOW")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// CHALLENGEWINDOW is a free data retrieval call binding the contract method 0xd62aad29.
//
// Solidity: function CHALLENGE_WINDOW() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) CHALLENGEWINDOW() (*big.Int, error) {
	return _PokerEscrow.Contract.CHALLENGEWINDOW(&_PokerEscrow.CallOpts)
}

// CHALLENGEWINDOW is a free data retrieval call binding the contract method 0xd62aad29.
//
// Solidity: function CHALLENGE_WINDOW() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) CHALLENGEWINDOW() (*big.Int, error) {
	return _PokerEscrow.Contract.CHALLENGEWINDOW(&_PokerEscrow.CallOpts)
}

// SETTLEMENTDEADLINE is a free data retrieval call binding the contract method 0xd687cd62.
//
// Solidity: function SETTLEMENT_DEADLINE() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) SETTLEMENTDEADLINE(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "SETTLEMENT_DEADLINE")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// SETTLEMENTDEADLINE is a free data retrieval call binding the contract method 0xd687cd62.
//
// Solidity: function SETTLEMENT_DEADLINE() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) SETTLEMENTDEADLINE() (*big.Int, error) {
	return _PokerEscrow.Contract.SETTLEMENTDEADLINE(&_PokerEscrow.CallOpts)
}

// SETTLEMENTDEADLINE is a free data retrieval call binding the contract method 0xd687cd62.
//
// Solidity: function SETTLEMENT_DEADLINE() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) SETTLEMENTDEADLINE() (*big.Int, error) {
	return _PokerEscrow.Contract.SETTLEMENTDEADLINE(&_PokerEscrow.CallOpts)
}

// SIGTHRESHOLDDEN is a free data retrieval call binding the contract method 0x1560eedd.
//
// Solidity: function SIG_THRESHOLD_DEN() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) SIGTHRESHOLDDEN(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "SIG_THRESHOLD_DEN")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// SIGTHRESHOLDDEN is a free data retrieval call binding the contract method 0x1560eedd.
//
// Solidity: function SIG_THRESHOLD_DEN() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) SIGTHRESHOLDDEN() (*big.Int, error) {
	return _PokerEscrow.Contract.SIGTHRESHOLDDEN(&_PokerEscrow.CallOpts)
}

// SIGTHRESHOLDDEN is a free data retrieval call binding the contract method 0x1560eedd.
//
// Solidity: function SIG_THRESHOLD_DEN() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) SIGTHRESHOLDDEN() (*big.Int, error) {
	return _PokerEscrow.Contract.SIGTHRESHOLDDEN(&_PokerEscrow.CallOpts)
}

// SIGTHRESHOLDNUM is a free data retrieval call binding the contract method 0x59eff2e1.
//
// Solidity: function SIG_THRESHOLD_NUM() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) SIGTHRESHOLDNUM(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "SIG_THRESHOLD_NUM")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// SIGTHRESHOLDNUM is a free data retrieval call binding the contract method 0x59eff2e1.
//
// Solidity: function SIG_THRESHOLD_NUM() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) SIGTHRESHOLDNUM() (*big.Int, error) {
	return _PokerEscrow.Contract.SIGTHRESHOLDNUM(&_PokerEscrow.CallOpts)
}

// SIGTHRESHOLDNUM is a free data retrieval call binding the contract method 0x59eff2e1.
//
// Solidity: function SIG_THRESHOLD_NUM() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) SIGTHRESHOLDNUM() (*big.Int, error) {
	return _PokerEscrow.Contract.SIGTHRESHOLDNUM(&_PokerEscrow.CallOpts)
}

// SLASHBURNBPS is a free data retrieval call binding the contract method 0xb28b6f33.
//
// Solidity: function SLASH_BURN_BPS() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) SLASHBURNBPS(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "SLASH_BURN_BPS")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// SLASHBURNBPS is a free data retrieval call binding the contract method 0xb28b6f33.
//
// Solidity: function SLASH_BURN_BPS() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) SLASHBURNBPS() (*big.Int, error) {
	return _PokerEscrow.Contract.SLASHBURNBPS(&_PokerEscrow.CallOpts)
}

// SLASHBURNBPS is a free data retrieval call binding the contract method 0xb28b6f33.
//
// Solidity: function SLASH_BURN_BPS() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) SLASHBURNBPS() (*big.Int, error) {
	return _PokerEscrow.Contract.SLASHBURNBPS(&_PokerEscrow.CallOpts)
}

// GameStartBlock is a free data retrieval call binding the contract method 0x3804c73d.
//
// Solidity: function gameStartBlock() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) GameStartBlock(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "gameStartBlock")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GameStartBlock is a free data retrieval call binding the contract method 0x3804c73d.
//
// Solidity: function gameStartBlock() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) GameStartBlock() (*big.Int, error) {
	return _PokerEscrow.Contract.GameStartBlock(&_PokerEscrow.CallOpts)
}

// GameStartBlock is a free data retrieval call binding the contract method 0x3804c73d.
//
// Solidity: function gameStartBlock() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) GameStartBlock() (*big.Int, error) {
	return _PokerEscrow.Contract.GameStartBlock(&_PokerEscrow.CallOpts)
}

// MaxSeats is a free data retrieval call binding the contract method 0x905d3f80.
//
// Solidity: function maxSeats() view returns(uint8)
func (_PokerEscrow *PokerEscrowCaller) MaxSeats(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "maxSeats")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// MaxSeats is a free data retrieval call binding the contract method 0x905d3f80.
//
// Solidity: function maxSeats() view returns(uint8)
func (_PokerEscrow *PokerEscrowSession) MaxSeats() (uint8, error) {
	return _PokerEscrow.Contract.MaxSeats(&_PokerEscrow.CallOpts)
}

// MaxSeats is a free data retrieval call binding the contract method 0x905d3f80.
//
// Solidity: function maxSeats() view returns(uint8)
func (_PokerEscrow *PokerEscrowCallerSession) MaxSeats() (uint8, error) {
	return _PokerEscrow.Contract.MaxSeats(&_PokerEscrow.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_PokerEscrow *PokerEscrowCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_PokerEscrow *PokerEscrowSession) Owner() (common.Address, error) {
	return _PokerEscrow.Contract.Owner(&_PokerEscrow.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_PokerEscrow *PokerEscrowCallerSession) Owner() (common.Address, error) {
	return _PokerEscrow.Contract.Owner(&_PokerEscrow.CallOpts)
}

// PlayerAt is a free data retrieval call binding the contract method 0xe0802c64.
//
// Solidity: function playerAt(uint8 seat) view returns(address addr, string peerID, uint256 buyIn, bool withdrawn, bool slashed)
func (_PokerEscrow *PokerEscrowCaller) PlayerAt(opts *bind.CallOpts, seat uint8) (struct {
	Addr      common.Address
	PeerID    string
	BuyIn     *big.Int
	Withdrawn bool
	Slashed   bool
}, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "playerAt", seat)

	outstruct := new(struct {
		Addr      common.Address
		PeerID    string
		BuyIn     *big.Int
		Withdrawn bool
		Slashed   bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Addr = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.PeerID = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.BuyIn = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.Withdrawn = *abi.ConvertType(out[3], new(bool)).(*bool)
	outstruct.Slashed = *abi.ConvertType(out[4], new(bool)).(*bool)

	return *outstruct, err

}

// PlayerAt is a free data retrieval call binding the contract method 0xe0802c64.
//
// Solidity: function playerAt(uint8 seat) view returns(address addr, string peerID, uint256 buyIn, bool withdrawn, bool slashed)
func (_PokerEscrow *PokerEscrowSession) PlayerAt(seat uint8) (struct {
	Addr      common.Address
	PeerID    string
	BuyIn     *big.Int
	Withdrawn bool
	Slashed   bool
}, error) {
	return _PokerEscrow.Contract.PlayerAt(&_PokerEscrow.CallOpts, seat)
}

// PlayerAt is a free data retrieval call binding the contract method 0xe0802c64.
//
// Solidity: function playerAt(uint8 seat) view returns(address addr, string peerID, uint256 buyIn, bool withdrawn, bool slashed)
func (_PokerEscrow *PokerEscrowCallerSession) PlayerAt(seat uint8) (struct {
	Addr      common.Address
	PeerID    string
	BuyIn     *big.Int
	Withdrawn bool
	Slashed   bool
}, error) {
	return _PokerEscrow.Contract.PlayerAt(&_PokerEscrow.CallOpts, seat)
}

// PlayerCount is a free data retrieval call binding the contract method 0x302bcc57.
//
// Solidity: function playerCount() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) PlayerCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "playerCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// PlayerCount is a free data retrieval call binding the contract method 0x302bcc57.
//
// Solidity: function playerCount() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) PlayerCount() (*big.Int, error) {
	return _PokerEscrow.Contract.PlayerCount(&_PokerEscrow.CallOpts)
}

// PlayerCount is a free data retrieval call binding the contract method 0x302bcc57.
//
// Solidity: function playerCount() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) PlayerCount() (*big.Int, error) {
	return _PokerEscrow.Contract.PlayerCount(&_PokerEscrow.CallOpts)
}

// Players is a free data retrieval call binding the contract method 0xf71d96cb.
//
// Solidity: function players(uint256 ) view returns(address addr, string peerID, uint256 buyIn, bool withdrawn, bool slashed)
func (_PokerEscrow *PokerEscrowCaller) Players(opts *bind.CallOpts, arg0 *big.Int) (struct {
	Addr      common.Address
	PeerID    string
	BuyIn     *big.Int
	Withdrawn bool
	Slashed   bool
}, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "players", arg0)

	outstruct := new(struct {
		Addr      common.Address
		PeerID    string
		BuyIn     *big.Int
		Withdrawn bool
		Slashed   bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Addr = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.PeerID = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.BuyIn = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.Withdrawn = *abi.ConvertType(out[3], new(bool)).(*bool)
	outstruct.Slashed = *abi.ConvertType(out[4], new(bool)).(*bool)

	return *outstruct, err

}

// Players is a free data retrieval call binding the contract method 0xf71d96cb.
//
// Solidity: function players(uint256 ) view returns(address addr, string peerID, uint256 buyIn, bool withdrawn, bool slashed)
func (_PokerEscrow *PokerEscrowSession) Players(arg0 *big.Int) (struct {
	Addr      common.Address
	PeerID    string
	BuyIn     *big.Int
	Withdrawn bool
	Slashed   bool
}, error) {
	return _PokerEscrow.Contract.Players(&_PokerEscrow.CallOpts, arg0)
}

// Players is a free data retrieval call binding the contract method 0xf71d96cb.
//
// Solidity: function players(uint256 ) view returns(address addr, string peerID, uint256 buyIn, bool withdrawn, bool slashed)
func (_PokerEscrow *PokerEscrowCallerSession) Players(arg0 *big.Int) (struct {
	Addr      common.Address
	PeerID    string
	BuyIn     *big.Int
	Withdrawn bool
	Slashed   bool
}, error) {
	return _PokerEscrow.Contract.Players(&_PokerEscrow.CallOpts, arg0)
}

// RequiredSignatures is a free data retrieval call binding the contract method 0x8d068043.
//
// Solidity: function requiredSignatures() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) RequiredSignatures(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "requiredSignatures")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RequiredSignatures is a free data retrieval call binding the contract method 0x8d068043.
//
// Solidity: function requiredSignatures() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) RequiredSignatures() (*big.Int, error) {
	return _PokerEscrow.Contract.RequiredSignatures(&_PokerEscrow.CallOpts)
}

// RequiredSignatures is a free data retrieval call binding the contract method 0x8d068043.
//
// Solidity: function requiredSignatures() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) RequiredSignatures() (*big.Int, error) {
	return _PokerEscrow.Contract.RequiredSignatures(&_PokerEscrow.CallOpts)
}

// SeatOf is a free data retrieval call binding the contract method 0xb9cd92c3.
//
// Solidity: function seatOf(address ) view returns(uint8)
func (_PokerEscrow *PokerEscrowCaller) SeatOf(opts *bind.CallOpts, arg0 common.Address) (uint8, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "seatOf", arg0)

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// SeatOf is a free data retrieval call binding the contract method 0xb9cd92c3.
//
// Solidity: function seatOf(address ) view returns(uint8)
func (_PokerEscrow *PokerEscrowSession) SeatOf(arg0 common.Address) (uint8, error) {
	return _PokerEscrow.Contract.SeatOf(&_PokerEscrow.CallOpts, arg0)
}

// SeatOf is a free data retrieval call binding the contract method 0xb9cd92c3.
//
// Solidity: function seatOf(address ) view returns(uint8)
func (_PokerEscrow *PokerEscrowCallerSession) SeatOf(arg0 common.Address) (uint8, error) {
	return _PokerEscrow.Contract.SeatOf(&_PokerEscrow.CallOpts, arg0)
}

// SettlementBlock is a free data retrieval call binding the contract method 0x8240c400.
//
// Solidity: function settlementBlock() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) SettlementBlock(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "settlementBlock")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// SettlementBlock is a free data retrieval call binding the contract method 0x8240c400.
//
// Solidity: function settlementBlock() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) SettlementBlock() (*big.Int, error) {
	return _PokerEscrow.Contract.SettlementBlock(&_PokerEscrow.CallOpts)
}

// SettlementBlock is a free data retrieval call binding the contract method 0x8240c400.
//
// Solidity: function settlementBlock() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) SettlementBlock() (*big.Int, error) {
	return _PokerEscrow.Contract.SettlementBlock(&_PokerEscrow.CallOpts)
}

// SlashEvidence is a free data retrieval call binding the contract method 0xdbf15385.
//
// Solidity: function slashEvidence(address ) view returns(bytes)
func (_PokerEscrow *PokerEscrowCaller) SlashEvidence(opts *bind.CallOpts, arg0 common.Address) ([]byte, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "slashEvidence", arg0)

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// SlashEvidence is a free data retrieval call binding the contract method 0xdbf15385.
//
// Solidity: function slashEvidence(address ) view returns(bytes)
func (_PokerEscrow *PokerEscrowSession) SlashEvidence(arg0 common.Address) ([]byte, error) {
	return _PokerEscrow.Contract.SlashEvidence(&_PokerEscrow.CallOpts, arg0)
}

// SlashEvidence is a free data retrieval call binding the contract method 0xdbf15385.
//
// Solidity: function slashEvidence(address ) view returns(bytes)
func (_PokerEscrow *PokerEscrowCallerSession) SlashEvidence(arg0 common.Address) ([]byte, error) {
	return _PokerEscrow.Contract.SlashEvidence(&_PokerEscrow.CallOpts, arg0)
}

// State is a free data retrieval call binding the contract method 0xc19d93fb.
//
// Solidity: function state() view returns(uint8)
func (_PokerEscrow *PokerEscrowCaller) State(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "state")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// State is a free data retrieval call binding the contract method 0xc19d93fb.
//
// Solidity: function state() view returns(uint8)
func (_PokerEscrow *PokerEscrowSession) State() (uint8, error) {
	return _PokerEscrow.Contract.State(&_PokerEscrow.CallOpts)
}

// State is a free data retrieval call binding the contract method 0xc19d93fb.
//
// Solidity: function state() view returns(uint8)
func (_PokerEscrow *PokerEscrowCallerSession) State() (uint8, error) {
	return _PokerEscrow.Contract.State(&_PokerEscrow.CallOpts)
}

// StateRoot is a free data retrieval call binding the contract method 0x9588eca2.
//
// Solidity: function stateRoot() view returns(bytes32)
func (_PokerEscrow *PokerEscrowCaller) StateRoot(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "stateRoot")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// StateRoot is a free data retrieval call binding the contract method 0x9588eca2.
//
// Solidity: function stateRoot() view returns(bytes32)
func (_PokerEscrow *PokerEscrowSession) StateRoot() ([32]byte, error) {
	return _PokerEscrow.Contract.StateRoot(&_PokerEscrow.CallOpts)
}

// StateRoot is a free data retrieval call binding the contract method 0x9588eca2.
//
// Solidity: function stateRoot() view returns(bytes32)
func (_PokerEscrow *PokerEscrowCallerSession) StateRoot() ([32]byte, error) {
	return _PokerEscrow.Contract.StateRoot(&_PokerEscrow.CallOpts)
}

// TableID is a free data retrieval call binding the contract method 0xff7d473b.
//
// Solidity: function tableID() view returns(string)
func (_PokerEscrow *PokerEscrowCaller) TableID(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "tableID")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// TableID is a free data retrieval call binding the contract method 0xff7d473b.
//
// Solidity: function tableID() view returns(string)
func (_PokerEscrow *PokerEscrowSession) TableID() (string, error) {
	return _PokerEscrow.Contract.TableID(&_PokerEscrow.CallOpts)
}

// TableID is a free data retrieval call binding the contract method 0xff7d473b.
//
// Solidity: function tableID() view returns(string)
func (_PokerEscrow *PokerEscrowCallerSession) TableID() (string, error) {
	return _PokerEscrow.Contract.TableID(&_PokerEscrow.CallOpts)
}

// TotalEscrow is a free data retrieval call binding the contract method 0xa3d89844.
//
// Solidity: function totalEscrow() view returns(uint256)
func (_PokerEscrow *PokerEscrowCaller) TotalEscrow(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PokerEscrow.contract.Call(opts, &out, "totalEscrow")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalEscrow is a free data retrieval call binding the contract method 0xa3d89844.
//
// Solidity: function totalEscrow() view returns(uint256)
func (_PokerEscrow *PokerEscrowSession) TotalEscrow() (*big.Int, error) {
	return _PokerEscrow.Contract.TotalEscrow(&_PokerEscrow.CallOpts)
}

// TotalEscrow is a free data retrieval call binding the contract method 0xa3d89844.
//
// Solidity: function totalEscrow() view returns(uint256)
func (_PokerEscrow *PokerEscrowCallerSession) TotalEscrow() (*big.Int, error) {
	return _PokerEscrow.Contract.TotalEscrow(&_PokerEscrow.CallOpts)
}

// JoinTable is a paid mutator transaction binding the contract method 0x098d7d76.
//
// Solidity: function joinTable(string peerID) payable returns()
func (_PokerEscrow *PokerEscrowTransactor) JoinTable(opts *bind.TransactOpts, peerID string) (*types.Transaction, error) {
	return _PokerEscrow.contract.Transact(opts, "joinTable", peerID)
}

// JoinTable is a paid mutator transaction binding the contract method 0x098d7d76.
//
// Solidity: function joinTable(string peerID) payable returns()
func (_PokerEscrow *PokerEscrowSession) JoinTable(peerID string) (*types.Transaction, error) {
	return _PokerEscrow.Contract.JoinTable(&_PokerEscrow.TransactOpts, peerID)
}

// JoinTable is a paid mutator transaction binding the contract method 0x098d7d76.
//
// Solidity: function joinTable(string peerID) payable returns()
func (_PokerEscrow *PokerEscrowTransactorSession) JoinTable(peerID string) (*types.Transaction, error) {
	return _PokerEscrow.Contract.JoinTable(&_PokerEscrow.TransactOpts, peerID)
}

// MarkAbandoned is a paid mutator transaction binding the contract method 0x87de9603.
//
// Solidity: function markAbandoned() returns()
func (_PokerEscrow *PokerEscrowTransactor) MarkAbandoned(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PokerEscrow.contract.Transact(opts, "markAbandoned")
}

// MarkAbandoned is a paid mutator transaction binding the contract method 0x87de9603.
//
// Solidity: function markAbandoned() returns()
func (_PokerEscrow *PokerEscrowSession) MarkAbandoned() (*types.Transaction, error) {
	return _PokerEscrow.Contract.MarkAbandoned(&_PokerEscrow.TransactOpts)
}

// MarkAbandoned is a paid mutator transaction binding the contract method 0x87de9603.
//
// Solidity: function markAbandoned() returns()
func (_PokerEscrow *PokerEscrowTransactorSession) MarkAbandoned() (*types.Transaction, error) {
	return _PokerEscrow.Contract.MarkAbandoned(&_PokerEscrow.TransactOpts)
}

// Refund is a paid mutator transaction binding the contract method 0x590e1ae3.
//
// Solidity: function refund() returns()
func (_PokerEscrow *PokerEscrowTransactor) Refund(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PokerEscrow.contract.Transact(opts, "refund")
}

// Refund is a paid mutator transaction binding the contract method 0x590e1ae3.
//
// Solidity: function refund() returns()
func (_PokerEscrow *PokerEscrowSession) Refund() (*types.Transaction, error) {
	return _PokerEscrow.Contract.Refund(&_PokerEscrow.TransactOpts)
}

// Refund is a paid mutator transaction binding the contract method 0x590e1ae3.
//
// Solidity: function refund() returns()
func (_PokerEscrow *PokerEscrowTransactorSession) Refund() (*types.Transaction, error) {
	return _PokerEscrow.Contract.Refund(&_PokerEscrow.TransactOpts)
}

// ReportOutcome is a paid mutator transaction binding the contract method 0x0cea58db.
//
// Solidity: function reportOutcome(int256[] payoutDeltas, bytes32 _stateRoot, bytes[] signatures, uint256 handNum) returns()
func (_PokerEscrow *PokerEscrowTransactor) ReportOutcome(opts *bind.TransactOpts, payoutDeltas []*big.Int, _stateRoot [32]byte, signatures [][]byte, handNum *big.Int) (*types.Transaction, error) {
	return _PokerEscrow.contract.Transact(opts, "reportOutcome", payoutDeltas, _stateRoot, signatures, handNum)
}

// ReportOutcome is a paid mutator transaction binding the contract method 0x0cea58db.
//
// Solidity: function reportOutcome(int256[] payoutDeltas, bytes32 _stateRoot, bytes[] signatures, uint256 handNum) returns()
func (_PokerEscrow *PokerEscrowSession) ReportOutcome(payoutDeltas []*big.Int, _stateRoot [32]byte, signatures [][]byte, handNum *big.Int) (*types.Transaction, error) {
	return _PokerEscrow.Contract.ReportOutcome(&_PokerEscrow.TransactOpts, payoutDeltas, _stateRoot, signatures, handNum)
}

// ReportOutcome is a paid mutator transaction binding the contract method 0x0cea58db.
//
// Solidity: function reportOutcome(int256[] payoutDeltas, bytes32 _stateRoot, bytes[] signatures, uint256 handNum) returns()
func (_PokerEscrow *PokerEscrowTransactorSession) ReportOutcome(payoutDeltas []*big.Int, _stateRoot [32]byte, signatures [][]byte, handNum *big.Int) (*types.Transaction, error) {
	return _PokerEscrow.Contract.ReportOutcome(&_PokerEscrow.TransactOpts, payoutDeltas, _stateRoot, signatures, handNum)
}

// SubmitDisput is a paid mutator transaction binding the contract method 0xdda98ef6.
//
// Solidity: function submitDisput(address accused, string reason, bytes evidence, bytes accuserSig) returns()
func (_PokerEscrow *PokerEscrowTransactor) SubmitDisput(opts *bind.TransactOpts, accused common.Address, reason string, evidence []byte, accuserSig []byte) (*types.Transaction, error) {
	return _PokerEscrow.contract.Transact(opts, "submitDisput", accused, reason, evidence, accuserSig)
}

// SubmitDisput is a paid mutator transaction binding the contract method 0xdda98ef6.
//
// Solidity: function submitDisput(address accused, string reason, bytes evidence, bytes accuserSig) returns()
func (_PokerEscrow *PokerEscrowSession) SubmitDisput(accused common.Address, reason string, evidence []byte, accuserSig []byte) (*types.Transaction, error) {
	return _PokerEscrow.Contract.SubmitDisput(&_PokerEscrow.TransactOpts, accused, reason, evidence, accuserSig)
}

// SubmitDisput is a paid mutator transaction binding the contract method 0xdda98ef6.
//
// Solidity: function submitDisput(address accused, string reason, bytes evidence, bytes accuserSig) returns()
func (_PokerEscrow *PokerEscrowTransactorSession) SubmitDisput(accused common.Address, reason string, evidence []byte, accuserSig []byte) (*types.Transaction, error) {
	return _PokerEscrow.Contract.SubmitDisput(&_PokerEscrow.TransactOpts, accused, reason, evidence, accuserSig)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_PokerEscrow *PokerEscrowTransactor) Receive(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PokerEscrow.contract.RawTransact(opts, nil) // calldata is disallowed for receive function
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_PokerEscrow *PokerEscrowSession) Receive() (*types.Transaction, error) {
	return _PokerEscrow.Contract.Receive(&_PokerEscrow.TransactOpts)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_PokerEscrow *PokerEscrowTransactorSession) Receive() (*types.Transaction, error) {
	return _PokerEscrow.Contract.Receive(&_PokerEscrow.TransactOpts)
}

// PokerEscrowAbandonedIterator is returned from FilterAbandoned and is used to iterate over the raw logs and unpacked data for Abandoned events raised by the PokerEscrow contract.
type PokerEscrowAbandonedIterator struct {
	Event *PokerEscrowAbandoned // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowAbandonedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowAbandoned)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowAbandoned)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowAbandonedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowAbandonedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowAbandoned represents a Abandoned event raised by the PokerEscrow contract.
type PokerEscrowAbandoned struct {
	BlockNumber *big.Int
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterAbandoned is a free log retrieval operation binding the contract event 0x4a09bc23edc2a2e0d70a566d0938a1d4561b8acc140cf5d0009fd64c24f30714.
//
// Solidity: event Abandoned(uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) FilterAbandoned(opts *bind.FilterOpts) (*PokerEscrowAbandonedIterator, error) {

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "Abandoned")
	if err != nil {
		return nil, err
	}
	return &PokerEscrowAbandonedIterator{contract: _PokerEscrow.contract, event: "Abandoned", logs: logs, sub: sub}, nil
}

// WatchAbandoned is a free log subscription operation binding the contract event 0x4a09bc23edc2a2e0d70a566d0938a1d4561b8acc140cf5d0009fd64c24f30714.
//
// Solidity: event Abandoned(uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) WatchAbandoned(opts *bind.WatchOpts, sink chan<- *PokerEscrowAbandoned) (event.Subscription, error) {

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "Abandoned")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowAbandoned)
				if err := _PokerEscrow.contract.UnpackLog(event, "Abandoned", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseAbandoned is a log parse operation binding the contract event 0x4a09bc23edc2a2e0d70a566d0938a1d4561b8acc140cf5d0009fd64c24f30714.
//
// Solidity: event Abandoned(uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) ParseAbandoned(log types.Log) (*PokerEscrowAbandoned, error) {
	event := new(PokerEscrowAbandoned)
	if err := _PokerEscrow.contract.UnpackLog(event, "Abandoned", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PokerEscrowDisputeFiledIterator is returned from FilterDisputeFiled and is used to iterate over the raw logs and unpacked data for DisputeFiled events raised by the PokerEscrow contract.
type PokerEscrowDisputeFiledIterator struct {
	Event *PokerEscrowDisputeFiled // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowDisputeFiledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowDisputeFiled)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowDisputeFiled)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowDisputeFiledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowDisputeFiledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowDisputeFiled represents a DisputeFiled event raised by the PokerEscrow contract.
type PokerEscrowDisputeFiled struct {
	Filer   common.Address
	Accused common.Address
	Reason  string
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterDisputeFiled is a free log retrieval operation binding the contract event 0xe2e9b489dfa0e4bdf49f5b8892bac04d45be3184c4d1bcda0314c552ef997030.
//
// Solidity: event DisputeFiled(address indexed filer, address indexed accused, string reason)
func (_PokerEscrow *PokerEscrowFilterer) FilterDisputeFiled(opts *bind.FilterOpts, filer []common.Address, accused []common.Address) (*PokerEscrowDisputeFiledIterator, error) {

	var filerRule []interface{}
	for _, filerItem := range filer {
		filerRule = append(filerRule, filerItem)
	}
	var accusedRule []interface{}
	for _, accusedItem := range accused {
		accusedRule = append(accusedRule, accusedItem)
	}

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "DisputeFiled", filerRule, accusedRule)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowDisputeFiledIterator{contract: _PokerEscrow.contract, event: "DisputeFiled", logs: logs, sub: sub}, nil
}

// WatchDisputeFiled is a free log subscription operation binding the contract event 0xe2e9b489dfa0e4bdf49f5b8892bac04d45be3184c4d1bcda0314c552ef997030.
//
// Solidity: event DisputeFiled(address indexed filer, address indexed accused, string reason)
func (_PokerEscrow *PokerEscrowFilterer) WatchDisputeFiled(opts *bind.WatchOpts, sink chan<- *PokerEscrowDisputeFiled, filer []common.Address, accused []common.Address) (event.Subscription, error) {

	var filerRule []interface{}
	for _, filerItem := range filer {
		filerRule = append(filerRule, filerItem)
	}
	var accusedRule []interface{}
	for _, accusedItem := range accused {
		accusedRule = append(accusedRule, accusedItem)
	}

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "DisputeFiled", filerRule, accusedRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowDisputeFiled)
				if err := _PokerEscrow.contract.UnpackLog(event, "DisputeFiled", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseDisputeFiled is a log parse operation binding the contract event 0xe2e9b489dfa0e4bdf49f5b8892bac04d45be3184c4d1bcda0314c552ef997030.
//
// Solidity: event DisputeFiled(address indexed filer, address indexed accused, string reason)
func (_PokerEscrow *PokerEscrowFilterer) ParseDisputeFiled(log types.Log) (*PokerEscrowDisputeFiled, error) {
	event := new(PokerEscrowDisputeFiled)
	if err := _PokerEscrow.contract.UnpackLog(event, "DisputeFiled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PokerEscrowGameStartedIterator is returned from FilterGameStarted and is used to iterate over the raw logs and unpacked data for GameStarted events raised by the PokerEscrow contract.
type PokerEscrowGameStartedIterator struct {
	Event *PokerEscrowGameStarted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowGameStartedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowGameStarted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowGameStarted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowGameStartedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowGameStartedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowGameStarted represents a GameStarted event raised by the PokerEscrow contract.
type PokerEscrowGameStarted struct {
	BlockNumber *big.Int
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterGameStarted is a free log retrieval operation binding the contract event 0x50ad08f58a27f2851d7e3a1b3a6a46b290f2ce677e99642d30ff639721e77790.
//
// Solidity: event GameStarted(uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) FilterGameStarted(opts *bind.FilterOpts) (*PokerEscrowGameStartedIterator, error) {

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "GameStarted")
	if err != nil {
		return nil, err
	}
	return &PokerEscrowGameStartedIterator{contract: _PokerEscrow.contract, event: "GameStarted", logs: logs, sub: sub}, nil
}

// WatchGameStarted is a free log subscription operation binding the contract event 0x50ad08f58a27f2851d7e3a1b3a6a46b290f2ce677e99642d30ff639721e77790.
//
// Solidity: event GameStarted(uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) WatchGameStarted(opts *bind.WatchOpts, sink chan<- *PokerEscrowGameStarted) (event.Subscription, error) {

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "GameStarted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowGameStarted)
				if err := _PokerEscrow.contract.UnpackLog(event, "GameStarted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseGameStarted is a log parse operation binding the contract event 0x50ad08f58a27f2851d7e3a1b3a6a46b290f2ce677e99642d30ff639721e77790.
//
// Solidity: event GameStarted(uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) ParseGameStarted(log types.Log) (*PokerEscrowGameStarted, error) {
	event := new(PokerEscrowGameStarted)
	if err := _PokerEscrow.contract.UnpackLog(event, "GameStarted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PokerEscrowOutcomeReportedIterator is returned from FilterOutcomeReported and is used to iterate over the raw logs and unpacked data for OutcomeReported events raised by the PokerEscrow contract.
type PokerEscrowOutcomeReportedIterator struct {
	Event *PokerEscrowOutcomeReported // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowOutcomeReportedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowOutcomeReported)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowOutcomeReported)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowOutcomeReportedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowOutcomeReportedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowOutcomeReported represents a OutcomeReported event raised by the PokerEscrow contract.
type PokerEscrowOutcomeReported struct {
	StateRoot   [32]byte
	BlockNumber *big.Int
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterOutcomeReported is a free log retrieval operation binding the contract event 0x62f40d491352a7684b2dd5daa9c5d245fc431befad253c3cfdab1e1987c48c38.
//
// Solidity: event OutcomeReported(bytes32 stateRoot, uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) FilterOutcomeReported(opts *bind.FilterOpts) (*PokerEscrowOutcomeReportedIterator, error) {

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "OutcomeReported")
	if err != nil {
		return nil, err
	}
	return &PokerEscrowOutcomeReportedIterator{contract: _PokerEscrow.contract, event: "OutcomeReported", logs: logs, sub: sub}, nil
}

// WatchOutcomeReported is a free log subscription operation binding the contract event 0x62f40d491352a7684b2dd5daa9c5d245fc431befad253c3cfdab1e1987c48c38.
//
// Solidity: event OutcomeReported(bytes32 stateRoot, uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) WatchOutcomeReported(opts *bind.WatchOpts, sink chan<- *PokerEscrowOutcomeReported) (event.Subscription, error) {

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "OutcomeReported")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowOutcomeReported)
				if err := _PokerEscrow.contract.UnpackLog(event, "OutcomeReported", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOutcomeReported is a log parse operation binding the contract event 0x62f40d491352a7684b2dd5daa9c5d245fc431befad253c3cfdab1e1987c48c38.
//
// Solidity: event OutcomeReported(bytes32 stateRoot, uint256 blockNumber)
func (_PokerEscrow *PokerEscrowFilterer) ParseOutcomeReported(log types.Log) (*PokerEscrowOutcomeReported, error) {
	event := new(PokerEscrowOutcomeReported)
	if err := _PokerEscrow.contract.UnpackLog(event, "OutcomeReported", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PokerEscrowPayoutSentIterator is returned from FilterPayoutSent and is used to iterate over the raw logs and unpacked data for PayoutSent events raised by the PokerEscrow contract.
type PokerEscrowPayoutSentIterator struct {
	Event *PokerEscrowPayoutSent // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowPayoutSentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowPayoutSent)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowPayoutSent)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowPayoutSentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowPayoutSentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowPayoutSent represents a PayoutSent event raised by the PokerEscrow contract.
type PokerEscrowPayoutSent struct {
	Player common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterPayoutSent is a free log retrieval operation binding the contract event 0x6c114f03096d92b098f0f087d8fd4a756d362d86a9649a07142404e7fd1d77b5.
//
// Solidity: event PayoutSent(address indexed player, uint256 amount)
func (_PokerEscrow *PokerEscrowFilterer) FilterPayoutSent(opts *bind.FilterOpts, player []common.Address) (*PokerEscrowPayoutSentIterator, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "PayoutSent", playerRule)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowPayoutSentIterator{contract: _PokerEscrow.contract, event: "PayoutSent", logs: logs, sub: sub}, nil
}

// WatchPayoutSent is a free log subscription operation binding the contract event 0x6c114f03096d92b098f0f087d8fd4a756d362d86a9649a07142404e7fd1d77b5.
//
// Solidity: event PayoutSent(address indexed player, uint256 amount)
func (_PokerEscrow *PokerEscrowFilterer) WatchPayoutSent(opts *bind.WatchOpts, sink chan<- *PokerEscrowPayoutSent, player []common.Address) (event.Subscription, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "PayoutSent", playerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowPayoutSent)
				if err := _PokerEscrow.contract.UnpackLog(event, "PayoutSent", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParsePayoutSent is a log parse operation binding the contract event 0x6c114f03096d92b098f0f087d8fd4a756d362d86a9649a07142404e7fd1d77b5.
//
// Solidity: event PayoutSent(address indexed player, uint256 amount)
func (_PokerEscrow *PokerEscrowFilterer) ParsePayoutSent(log types.Log) (*PokerEscrowPayoutSent, error) {
	event := new(PokerEscrowPayoutSent)
	if err := _PokerEscrow.contract.UnpackLog(event, "PayoutSent", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PokerEscrowPlayerJoinedIterator is returned from FilterPlayerJoined and is used to iterate over the raw logs and unpacked data for PlayerJoined events raised by the PokerEscrow contract.
type PokerEscrowPlayerJoinedIterator struct {
	Event *PokerEscrowPlayerJoined // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowPlayerJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowPlayerJoined)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowPlayerJoined)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowPlayerJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowPlayerJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowPlayerJoined represents a PlayerJoined event raised by the PokerEscrow contract.
type PokerEscrowPlayerJoined struct {
	Player common.Address
	PeerID string
	Amount *big.Int
	Seat   uint8
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterPlayerJoined is a free log retrieval operation binding the contract event 0xdc368a5d879bee435c8a531083c19953a9786275f18866f225baeb38bef88893.
//
// Solidity: event PlayerJoined(address indexed player, string peerID, uint256 amount, uint8 seat)
func (_PokerEscrow *PokerEscrowFilterer) FilterPlayerJoined(opts *bind.FilterOpts, player []common.Address) (*PokerEscrowPlayerJoinedIterator, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "PlayerJoined", playerRule)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowPlayerJoinedIterator{contract: _PokerEscrow.contract, event: "PlayerJoined", logs: logs, sub: sub}, nil
}

// WatchPlayerJoined is a free log subscription operation binding the contract event 0xdc368a5d879bee435c8a531083c19953a9786275f18866f225baeb38bef88893.
//
// Solidity: event PlayerJoined(address indexed player, string peerID, uint256 amount, uint8 seat)
func (_PokerEscrow *PokerEscrowFilterer) WatchPlayerJoined(opts *bind.WatchOpts, sink chan<- *PokerEscrowPlayerJoined, player []common.Address) (event.Subscription, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "PlayerJoined", playerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowPlayerJoined)
				if err := _PokerEscrow.contract.UnpackLog(event, "PlayerJoined", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParsePlayerJoined is a log parse operation binding the contract event 0xdc368a5d879bee435c8a531083c19953a9786275f18866f225baeb38bef88893.
//
// Solidity: event PlayerJoined(address indexed player, string peerID, uint256 amount, uint8 seat)
func (_PokerEscrow *PokerEscrowFilterer) ParsePlayerJoined(log types.Log) (*PokerEscrowPlayerJoined, error) {
	event := new(PokerEscrowPlayerJoined)
	if err := _PokerEscrow.contract.UnpackLog(event, "PlayerJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PokerEscrowRefundedIterator is returned from FilterRefunded and is used to iterate over the raw logs and unpacked data for Refunded events raised by the PokerEscrow contract.
type PokerEscrowRefundedIterator struct {
	Event *PokerEscrowRefunded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowRefundedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowRefunded)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowRefunded)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowRefundedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowRefundedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowRefunded represents a Refunded event raised by the PokerEscrow contract.
type PokerEscrowRefunded struct {
	Player common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRefunded is a free log retrieval operation binding the contract event 0xd7dee2702d63ad89917b6a4da9981c90c4d24f8c2bdfd64c604ecae57d8d0651.
//
// Solidity: event Refunded(address indexed player, uint256 amount)
func (_PokerEscrow *PokerEscrowFilterer) FilterRefunded(opts *bind.FilterOpts, player []common.Address) (*PokerEscrowRefundedIterator, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "Refunded", playerRule)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowRefundedIterator{contract: _PokerEscrow.contract, event: "Refunded", logs: logs, sub: sub}, nil
}

// WatchRefunded is a free log subscription operation binding the contract event 0xd7dee2702d63ad89917b6a4da9981c90c4d24f8c2bdfd64c604ecae57d8d0651.
//
// Solidity: event Refunded(address indexed player, uint256 amount)
func (_PokerEscrow *PokerEscrowFilterer) WatchRefunded(opts *bind.WatchOpts, sink chan<- *PokerEscrowRefunded, player []common.Address) (event.Subscription, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "Refunded", playerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowRefunded)
				if err := _PokerEscrow.contract.UnpackLog(event, "Refunded", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRefunded is a log parse operation binding the contract event 0xd7dee2702d63ad89917b6a4da9981c90c4d24f8c2bdfd64c604ecae57d8d0651.
//
// Solidity: event Refunded(address indexed player, uint256 amount)
func (_PokerEscrow *PokerEscrowFilterer) ParseRefunded(log types.Log) (*PokerEscrowRefunded, error) {
	event := new(PokerEscrowRefunded)
	if err := _PokerEscrow.contract.UnpackLog(event, "Refunded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PokerEscrowSlashExecutedIterator is returned from FilterSlashExecuted and is used to iterate over the raw logs and unpacked data for SlashExecuted events raised by the PokerEscrow contract.
type PokerEscrowSlashExecutedIterator struct {
	Event *PokerEscrowSlashExecuted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PokerEscrowSlashExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PokerEscrowSlashExecuted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PokerEscrowSlashExecuted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PokerEscrowSlashExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PokerEscrowSlashExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PokerEscrowSlashExecuted represents a SlashExecuted event raised by the PokerEscrow contract.
type PokerEscrowSlashExecuted struct {
	Player        common.Address
	SlashedAmount *big.Int
	BurnedAmount  *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterSlashExecuted is a free log retrieval operation binding the contract event 0xe555f764b6ca0ad00001ba7c10f781c8356ec509b13db4a1b7d7b6b8ac69419c.
//
// Solidity: event SlashExecuted(address indexed player, uint256 slashedAmount, uint256 burnedAmount)
func (_PokerEscrow *PokerEscrowFilterer) FilterSlashExecuted(opts *bind.FilterOpts, player []common.Address) (*PokerEscrowSlashExecutedIterator, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.FilterLogs(opts, "SlashExecuted", playerRule)
	if err != nil {
		return nil, err
	}
	return &PokerEscrowSlashExecutedIterator{contract: _PokerEscrow.contract, event: "SlashExecuted", logs: logs, sub: sub}, nil
}

// WatchSlashExecuted is a free log subscription operation binding the contract event 0xe555f764b6ca0ad00001ba7c10f781c8356ec509b13db4a1b7d7b6b8ac69419c.
//
// Solidity: event SlashExecuted(address indexed player, uint256 slashedAmount, uint256 burnedAmount)
func (_PokerEscrow *PokerEscrowFilterer) WatchSlashExecuted(opts *bind.WatchOpts, sink chan<- *PokerEscrowSlashExecuted, player []common.Address) (event.Subscription, error) {

	var playerRule []interface{}
	for _, playerItem := range player {
		playerRule = append(playerRule, playerItem)
	}

	logs, sub, err := _PokerEscrow.contract.WatchLogs(opts, "SlashExecuted", playerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PokerEscrowSlashExecuted)
				if err := _PokerEscrow.contract.UnpackLog(event, "SlashExecuted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSlashExecuted is a log parse operation binding the contract event 0xe555f764b6ca0ad00001ba7c10f781c8356ec509b13db4a1b7d7b6b8ac69419c.
//
// Solidity: event SlashExecuted(address indexed player, uint256 slashedAmount, uint256 burnedAmount)
func (_PokerEscrow *PokerEscrowFilterer) ParseSlashExecuted(log types.Log) (*PokerEscrowSlashExecuted, error) {
	event := new(PokerEscrowSlashExecuted)
	if err := _PokerEscrow.contract.UnpackLog(event, "SlashExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
