package actors

import (
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"
)

const SectorSize = 1024

func init() {
	cbor.RegisterCborType(StorageMarketState{})
	cbor.RegisterCborType(CreateStorageMinerParams{})
	cbor.RegisterCborType(IsMinerParam{})
	cbor.RegisterCborType(PowerLookupParams{})
}

type StorageMarketActor struct{}

func (sma StorageMarketActor) Exports() []interface{} {
	return []interface{}{
		//0: sma.StorageMarketConstructor,
		1: sma.CreateStorageMiner,
		//2: sma.SlashConsensusFault,
		3: sma.UpdateStorage,
		4: sma.GetTotalStorage,
		5: sma.PowerLookup,
		6: sma.IsMiner,
		//7: sma.StorageCollateralForSize,
	}
}

type StorageMarketState struct {
	Miners       map[address.Address]struct{}
	TotalStorage types.BigInt
}

type CreateStorageMinerParams struct {
	Owner      address.Address
	Worker     address.Address
	SectorSize types.BigInt
	PeerID     peer.ID
}

func (sma StorageMarketActor) CreateStorageMiner(act *types.Actor, vmctx types.VMContext, params *CreateStorageMinerParams) (types.InvokeRet, error) {
	if !SupportedSectorSize(params.SectorSize) {
		//Fatal("Unsupported sector size")
		return types.InvokeRet{
			ReturnCode: 1,
		}, nil
	}

	encoded, err := CreateExecParams(StorageMinerCodeCid, &StorageMinerConstructorParams{
		Owner:      params.Owner,
		Worker:     params.Worker,
		SectorSize: params.SectorSize,
		PeerID:     params.PeerID,
	})
	if err != nil {
		return types.InvokeRet{}, err
	}

	ret, exit, err := vmctx.Send(InitActorAddress, 1, vmctx.Message().Value, encoded)
	if err != nil {
		return types.InvokeRet{}, err
	}

	if exit != 0 {
		return types.InvokeRet{
			ReturnCode: 2,
		}, nil
	}

	naddr, err := address.NewFromBytes(ret)
	if err != nil {
		return types.InvokeRet{}, err
	}

	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return types.InvokeRet{}, err
	}

	self.Miners[naddr] = struct{}{}

	nroot, err := vmctx.Storage().Put(self)
	if err != nil {
		return types.InvokeRet{}, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return types.InvokeRet{}, err
	}

	return types.InvokeRet{
		Result: naddr.Bytes(),
	}, nil
}

func SupportedSectorSize(ssize types.BigInt) bool {
	if ssize.Uint64() == SectorSize {
		return true
	}
	return false
}

type UpdateStorageParams struct {
	Delta types.BigInt
}

func (sma StorageMarketActor) UpdateStorage(act *types.Actor, vmctx types.VMContext, params *UpdateStorageParams) (types.InvokeRet, error) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return types.InvokeRet{}, err
	}

	_, ok := self.Miners[vmctx.Message().From]
	if !ok {
		//Fatal("update storage must only be called by a miner actor")
		return types.InvokeRet{
			ReturnCode: 1,
		}, nil
	}

	self.TotalStorage = types.BigAdd(self.TotalStorage, params.Delta)
	return types.InvokeRet{}, nil
}

func (sma StorageMarketActor) GetTotalStorage(act *types.Actor, vmctx types.VMContext, params *struct{}) (types.InvokeRet, error) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return types.InvokeRet{}, err
	}

	return types.InvokeRet{
		Result: self.TotalStorage.Bytes(),
	}, nil
}

type PowerLookupParams struct {
	Miner address.Address
}

func (sma StorageMarketActor) PowerLookup(act *types.Actor, vmctx types.VMContext, params *PowerLookupParams) (types.InvokeRet, error) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return types.InvokeRet{}, xerrors.Errorf("getting head: %w", err)
	}

	if _, ok := self.Miners[params.Miner]; !ok {
		//Fatal("miner not registered with storage market")
		return types.InvokeRet{
			ReturnCode: 1,
		}, nil
	}

	ret, code, err := vmctx.Send(params.Miner, 9, types.NewInt(0), EmptyStructCBOR)
	if err != nil {
		return types.InvokeRet{}, xerrors.Errorf("invoke Miner.GetPower: %w", err)
	}

	if code != 0 {
		return types.InvokeRet{
			// TODO: error handling... these codes really don't tell us what the problem is very well
			ReturnCode: code,
		}, nil
	}

	return types.InvokeRet{
		Result: ret,
	}, nil
}

type IsMinerParam struct {
	Addr address.Address
}

func (sma StorageMarketActor) IsMiner(act *types.Actor, vmctx types.VMContext, param *IsMinerParam) (types.InvokeRet, error) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return types.InvokeRet{}, err
	}
	_, ok := self.Miners[param.Addr]
	out, err := SerializeParams(ok)
	if err != nil {
		return types.InvokeRet{}, err
	}
	return types.InvokeRet{
		Result: out,
	}, nil
}