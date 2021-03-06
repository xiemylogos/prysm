package state

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/ethereumapis/eth/v1alpha1"
	"github.com/prysmaticlabs/go-bitfield"
	coreutils "github.com/prysmaticlabs/prysm/beacon-chain/core/state/stateutils"
	pbp2p "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/featureconfig"
	"github.com/prysmaticlabs/prysm/shared/hashutil"
	"github.com/prysmaticlabs/prysm/shared/memorypool"
)

// SetGenesisTime for the beacon state.
func (b *BeaconState) SetGenesisTime(val uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.GenesisTime = val
	b.markFieldAsDirty(genesisTime)
	return nil
}

// SetGenesisValidatorRoot for the beacon state.
func (b *BeaconState) SetGenesisValidatorRoot(val []byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.GenesisValidatorsRoot = val
	b.markFieldAsDirty(genesisValidatorRoot)
	return nil
}

// SetSlot for the beacon state.
func (b *BeaconState) SetSlot(val uint64) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Slot = val
	b.markFieldAsDirty(slot)
	return nil
}

// SetFork version for the beacon chain.
func (b *BeaconState) SetFork(val *pbp2p.Fork) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	fk, ok := proto.Clone(val).(*pbp2p.Fork)
	if !ok {
		return errors.New("proto.Clone did not return a fork proto")
	}
	b.state.Fork = fk
	b.markFieldAsDirty(fork)
	return nil
}

// SetLatestBlockHeader in the beacon state.
func (b *BeaconState) SetLatestBlockHeader(val *ethpb.BeaconBlockHeader) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.LatestBlockHeader = CopyBeaconBlockHeader(val)
	b.markFieldAsDirty(latestBlockHeader)
	return nil
}

// SetBlockRoots for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetBlockRoots(val [][]byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[blockRoots].refs--
	b.sharedFieldReferences[blockRoots] = &reference{refs: 1}

	b.state.BlockRoots = val
	b.markFieldAsDirty(blockRoots)
	return nil
}

// UpdateBlockRootAtIndex for the beacon state. This PR updates the randao mixes
// at a specific index to a new value.
func (b *BeaconState) UpdateBlockRootAtIndex(idx uint64, blockRoot [32]byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	if len(b.state.BlockRoots) <= int(idx) {
		return fmt.Errorf("invalid index provided %d", idx)
	}

	b.lock.RLock()
	r := b.state.BlockRoots
	if ref := b.sharedFieldReferences[blockRoots]; ref.refs > 1 {
		// Copy on write since this is a shared array.
		if featureconfig.Get().EnableStateRefCopy {
			r = make([][]byte, len(b.state.BlockRoots))
			copy(r, b.state.BlockRoots)
		} else {
			r = b.BlockRoots()
		}

		ref.MinusRef()
		b.sharedFieldReferences[blockRoots] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	// Must secure lock after copy or hit a deadlock.
	b.lock.Lock()
	defer b.lock.Unlock()

	r[idx] = blockRoot[:]
	b.state.BlockRoots = r

	b.markFieldAsDirty(blockRoots)
	b.AddDirtyIndices(blockRoots, []uint64{idx})
	return nil
}

// SetStateRoots for the beacon state. This PR updates the entire
// to a new value by overwriting the previous one.
func (b *BeaconState) SetStateRoots(val [][]byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[stateRoots].refs--
	b.sharedFieldReferences[stateRoots] = &reference{refs: 1}

	b.state.StateRoots = val
	b.markFieldAsDirty(stateRoots)
	b.rebuildTrie[stateRoots] = true
	return nil
}

// UpdateStateRootAtIndex for the beacon state. This PR updates the randao mixes
// at a specific index to a new value.
func (b *BeaconState) UpdateStateRootAtIndex(idx uint64, stateRoot [32]byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	if len(b.state.StateRoots) <= int(idx) {
		return errors.Errorf("invalid index provided %d", idx)
	}

	b.lock.RLock()
	// Check if we hold the only reference to the shared state roots slice.
	r := b.state.StateRoots
	if ref := b.sharedFieldReferences[stateRoots]; ref.refs > 1 {
		// Perform a copy since this is a shared reference and we don't want to mutate others.
		if featureconfig.Get().EnableStateRefCopy {
			r = make([][]byte, len(b.state.StateRoots))
			copy(r, b.state.StateRoots)
		} else {
			r = b.StateRoots()
		}

		ref.MinusRef()
		b.sharedFieldReferences[stateRoots] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	// Must secure lock after copy or hit a deadlock.
	b.lock.Lock()
	defer b.lock.Unlock()

	r[idx] = stateRoot[:]
	b.state.StateRoots = r

	b.markFieldAsDirty(stateRoots)
	b.AddDirtyIndices(stateRoots, []uint64{idx})
	return nil
}

// SetHistoricalRoots for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetHistoricalRoots(val [][]byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[historicalRoots].refs--
	b.sharedFieldReferences[historicalRoots] = &reference{refs: 1}

	b.state.HistoricalRoots = val
	b.markFieldAsDirty(historicalRoots)
	return nil
}

// SetEth1Data for the beacon state.
func (b *BeaconState) SetEth1Data(val *ethpb.Eth1Data) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Eth1Data = val
	b.markFieldAsDirty(eth1Data)
	return nil
}

// SetEth1DataVotes for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetEth1DataVotes(val []*ethpb.Eth1Data) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[eth1DataVotes].refs--
	b.sharedFieldReferences[eth1DataVotes] = &reference{refs: 1}

	b.state.Eth1DataVotes = val
	b.markFieldAsDirty(eth1DataVotes)
	b.rebuildTrie[eth1DataVotes] = true
	return nil
}

// AppendEth1DataVotes for the beacon state. This PR appends the new value
// to the the end of list.
func (b *BeaconState) AppendEth1DataVotes(val *ethpb.Eth1Data) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.RLock()
	votes := b.state.Eth1DataVotes
	if b.sharedFieldReferences[eth1DataVotes].refs > 1 {
		if featureconfig.Get().EnableStateRefCopy {
			votes = make([]*ethpb.Eth1Data, len(b.state.Eth1DataVotes))
			copy(votes, b.state.Eth1DataVotes)
		} else {
			votes = b.Eth1DataVotes()
		}
		b.sharedFieldReferences[eth1DataVotes].MinusRef()
		b.sharedFieldReferences[eth1DataVotes] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Eth1DataVotes = append(votes, val)
	b.markFieldAsDirty(eth1DataVotes)
	b.AddDirtyIndices(eth1DataVotes, []uint64{uint64(len(b.state.Eth1DataVotes) - 1)})
	return nil
}

// SetEth1DepositIndex for the beacon state.
func (b *BeaconState) SetEth1DepositIndex(val uint64) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Eth1DepositIndex = val
	b.markFieldAsDirty(eth1DepositIndex)
	return nil
}

// SetValidators for the beacon state. This PR updates the entire
// to a new value by overwriting the previous one.
func (b *BeaconState) SetValidators(val []*ethpb.Validator) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Validators = val
	b.sharedFieldReferences[validators].refs--
	b.sharedFieldReferences[validators] = &reference{refs: 1}
	b.markFieldAsDirty(validators)
	b.rebuildTrie[validators] = true
	b.valIdxMap = coreutils.ValidatorIndexMap(b.state.Validators)
	return nil
}

// ApplyToEveryValidator applies the provided callback function to each validator in the
// validator registry.
func (b *BeaconState) ApplyToEveryValidator(checker func(idx int, val *ethpb.Validator) (bool, error),
	mutator func(idx int, val *ethpb.Validator) error) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.RLock()
	v := b.state.Validators
	if ref := b.sharedFieldReferences[validators]; ref.refs > 1 {
		// Perform a copy since this is a shared reference and we don't want to mutate others.
		if featureconfig.Get().EnableStateRefCopy {
			v = make([]*ethpb.Validator, len(b.state.Validators))
			copy(v, b.state.Validators)
		} else {
			v = b.Validators()
		}
		ref.MinusRef()
		b.sharedFieldReferences[validators] = &reference{refs: 1}
	}
	b.lock.RUnlock()
	var changedVals []uint64
	for i, val := range v {
		changed, err := checker(i, val)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if featureconfig.Get().EnableStateRefCopy {
			// copy if changing a reference
			val = CopyValidator(val)
		}
		err = mutator(i, val)
		if err != nil {
			return err
		}
		changedVals = append(changedVals, uint64(i))
		v[i] = val
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Validators = v
	b.markFieldAsDirty(validators)
	b.AddDirtyIndices(validators, changedVals)

	return nil
}

// UpdateValidatorAtIndex for the beacon state. This PR updates the randao mixes
// at a specific index to a new value.
func (b *BeaconState) UpdateValidatorAtIndex(idx uint64, val *ethpb.Validator) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	if len(b.state.Validators) <= int(idx) {
		return errors.Errorf("invalid index provided %d", idx)
	}

	b.lock.RLock()
	v := b.state.Validators
	if ref := b.sharedFieldReferences[validators]; ref.refs > 1 {
		// Perform a copy since this is a shared reference and we don't want to mutate others.
		if featureconfig.Get().EnableStateRefCopy {
			v = make([]*ethpb.Validator, len(b.state.Validators))
			copy(v, b.state.Validators)
		} else {
			v = b.Validators()
		}

		ref.MinusRef()
		b.sharedFieldReferences[validators] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	v[idx] = val
	b.state.Validators = v
	b.markFieldAsDirty(validators)
	b.AddDirtyIndices(validators, []uint64{idx})

	return nil
}

// SetValidatorIndexByPubkey updates the validator index mapping maintained internally to
// a given input 48-byte, public key.
func (b *BeaconState) SetValidatorIndexByPubkey(pubKey [48]byte, validatorIdx uint64) {
	// Copy on write since this is a shared map.
	m := b.validatorIndexMap()

	b.lock.Lock()
	defer b.lock.Unlock()

	m[pubKey] = validatorIdx
	b.valIdxMap = m
}

// SetBalances for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetBalances(val []uint64) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[balances].refs--
	b.sharedFieldReferences[balances] = &reference{refs: 1}

	b.state.Balances = val
	b.markFieldAsDirty(balances)
	return nil
}

// UpdateBalancesAtIndex for the beacon state. This method updates the balance
// at a specific index to a new value.
func (b *BeaconState) UpdateBalancesAtIndex(idx uint64, val uint64) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	if len(b.state.Balances) <= int(idx) {
		return errors.Errorf("invalid index provided %d", idx)
	}

	b.lock.RLock()
	bals := b.state.Balances
	if b.sharedFieldReferences[balances].refs > 1 {
		bals = b.Balances()
		b.sharedFieldReferences[balances].MinusRef()
		b.sharedFieldReferences[balances] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	bals[idx] = val
	b.state.Balances = bals
	b.markFieldAsDirty(balances)
	return nil
}

// SetRandaoMixes for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetRandaoMixes(val [][]byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[randaoMixes].refs--
	b.sharedFieldReferences[randaoMixes] = &reference{refs: 1}

	b.state.RandaoMixes = val
	b.markFieldAsDirty(randaoMixes)
	b.rebuildTrie[randaoMixes] = true
	return nil
}

// UpdateRandaoMixesAtIndex for the beacon state. This PR updates the randao mixes
// at a specific index to a new value.
func (b *BeaconState) UpdateRandaoMixesAtIndex(idx uint64, val []byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	if len(b.state.RandaoMixes) <= int(idx) {
		return errors.Errorf("invalid index provided %d", idx)
	}

	b.lock.RLock()
	mixes := b.state.RandaoMixes
	if refs := b.sharedFieldReferences[randaoMixes].refs; refs > 1 {
		if featureconfig.Get().EnableStateRefCopy {
			mixes = memorypool.GetDoubleByteSlice(len(b.state.RandaoMixes))
			copy(mixes, b.state.RandaoMixes)
		} else {
			mixes = b.RandaoMixes()
		}
		b.sharedFieldReferences[randaoMixes].MinusRef()
		b.sharedFieldReferences[randaoMixes] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	mixes[idx] = val
	b.state.RandaoMixes = mixes
	b.markFieldAsDirty(randaoMixes)
	b.AddDirtyIndices(randaoMixes, []uint64{idx})

	return nil
}

// SetSlashings for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetSlashings(val []uint64) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[slashings].refs--
	b.sharedFieldReferences[slashings] = &reference{refs: 1}

	b.state.Slashings = val
	b.markFieldAsDirty(slashings)
	return nil
}

// UpdateSlashingsAtIndex for the beacon state. This PR updates the randao mixes
// at a specific index to a new value.
func (b *BeaconState) UpdateSlashingsAtIndex(idx uint64, val uint64) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	if len(b.state.Slashings) <= int(idx) {
		return errors.Errorf("invalid index provided %d", idx)
	}
	b.lock.RLock()
	s := b.state.Slashings

	if b.sharedFieldReferences[slashings].refs > 1 {
		s = b.Slashings()
		b.sharedFieldReferences[slashings].MinusRef()
		b.sharedFieldReferences[slashings] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	s[idx] = val

	b.state.Slashings = s

	b.markFieldAsDirty(slashings)
	return nil
}

// SetPreviousEpochAttestations for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetPreviousEpochAttestations(val []*pbp2p.PendingAttestation) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[previousEpochAttestations].refs--
	b.sharedFieldReferences[previousEpochAttestations] = &reference{refs: 1}

	b.state.PreviousEpochAttestations = val
	b.markFieldAsDirty(previousEpochAttestations)
	b.rebuildTrie[previousEpochAttestations] = true
	return nil
}

// SetCurrentEpochAttestations for the beacon state. This PR updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetCurrentEpochAttestations(val []*pbp2p.PendingAttestation) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[currentEpochAttestations].refs--
	b.sharedFieldReferences[currentEpochAttestations] = &reference{refs: 1}

	b.state.CurrentEpochAttestations = val
	b.markFieldAsDirty(currentEpochAttestations)
	b.rebuildTrie[currentEpochAttestations] = true
	return nil
}

// AppendHistoricalRoots for the beacon state. This PR appends the new value
// to the the end of list.
func (b *BeaconState) AppendHistoricalRoots(root [32]byte) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.RLock()
	roots := b.state.HistoricalRoots
	if b.sharedFieldReferences[historicalRoots].refs > 1 {
		if featureconfig.Get().EnableStateRefCopy {
			roots = make([][]byte, len(b.state.HistoricalRoots))
			copy(roots, b.state.HistoricalRoots)
		} else {
			roots = b.HistoricalRoots()
		}
		b.sharedFieldReferences[historicalRoots].MinusRef()
		b.sharedFieldReferences[historicalRoots] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.HistoricalRoots = append(roots, root[:])
	b.markFieldAsDirty(historicalRoots)
	return nil
}

// AppendCurrentEpochAttestations for the beacon state. This PR appends the new value
// to the the end of list.
func (b *BeaconState) AppendCurrentEpochAttestations(val *pbp2p.PendingAttestation) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.RLock()

	atts := b.state.CurrentEpochAttestations
	if b.sharedFieldReferences[currentEpochAttestations].refs > 1 {
		if featureconfig.Get().EnableStateRefCopy {
			atts = make([]*pbp2p.PendingAttestation, len(b.state.CurrentEpochAttestations))
			copy(atts, b.state.CurrentEpochAttestations)
		} else {
			atts = b.CurrentEpochAttestations()
		}
		b.sharedFieldReferences[currentEpochAttestations].MinusRef()
		b.sharedFieldReferences[currentEpochAttestations] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.CurrentEpochAttestations = append(atts, val)
	b.markFieldAsDirty(currentEpochAttestations)
	b.dirtyIndices[currentEpochAttestations] = append(b.dirtyIndices[currentEpochAttestations], uint64(len(b.state.CurrentEpochAttestations)-1))
	return nil
}

// AppendPreviousEpochAttestations for the beacon state. This PR appends the new value
// to the the end of list.
func (b *BeaconState) AppendPreviousEpochAttestations(val *pbp2p.PendingAttestation) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.RLock()
	atts := b.state.PreviousEpochAttestations
	if b.sharedFieldReferences[previousEpochAttestations].refs > 1 {
		if featureconfig.Get().EnableStateRefCopy {
			atts = make([]*pbp2p.PendingAttestation, len(b.state.PreviousEpochAttestations))
			copy(atts, b.state.PreviousEpochAttestations)
		} else {
			atts = b.PreviousEpochAttestations()
		}
		b.sharedFieldReferences[previousEpochAttestations].MinusRef()
		b.sharedFieldReferences[previousEpochAttestations] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.PreviousEpochAttestations = append(atts, val)
	b.markFieldAsDirty(previousEpochAttestations)
	b.AddDirtyIndices(previousEpochAttestations, []uint64{uint64(len(b.state.PreviousEpochAttestations) - 1)})

	return nil
}

// AppendValidator for the beacon state. This PR appends the new value
// to the the end of list.
func (b *BeaconState) AppendValidator(val *ethpb.Validator) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.RLock()
	vals := b.state.Validators
	if b.sharedFieldReferences[validators].refs > 1 {
		if featureconfig.Get().EnableStateRefCopy {
			vals = make([]*ethpb.Validator, len(b.state.Validators))
			copy(vals, b.state.Validators)
		} else {
			vals = b.Validators()
		}
		b.sharedFieldReferences[validators].MinusRef()
		b.sharedFieldReferences[validators] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Validators = append(vals, val)
	b.markFieldAsDirty(validators)
	b.AddDirtyIndices(validators, []uint64{uint64(len(b.state.Validators) - 1)})
	b.valIdxMap = coreutils.ValidatorIndexMap(b.state.Validators)
	return nil
}

// AppendBalance for the beacon state. This PR appends the new value
// to the the end of list.
func (b *BeaconState) AppendBalance(bal uint64) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.RLock()

	bals := b.state.Balances
	if b.sharedFieldReferences[balances].refs > 1 {
		bals = b.Balances()
		b.sharedFieldReferences[balances].MinusRef()
		b.sharedFieldReferences[balances] = &reference{refs: 1}
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.Balances = append(bals, bal)
	b.markFieldAsDirty(balances)
	return nil
}

// SetJustificationBits for the beacon state.
func (b *BeaconState) SetJustificationBits(val bitfield.Bitvector4) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.JustificationBits = val
	b.markFieldAsDirty(justificationBits)
	return nil
}

// SetPreviousJustifiedCheckpoint for the beacon state.
func (b *BeaconState) SetPreviousJustifiedCheckpoint(val *ethpb.Checkpoint) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.PreviousJustifiedCheckpoint = val
	b.markFieldAsDirty(previousJustifiedCheckpoint)
	return nil
}

// SetCurrentJustifiedCheckpoint for the beacon state.
func (b *BeaconState) SetCurrentJustifiedCheckpoint(val *ethpb.Checkpoint) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.CurrentJustifiedCheckpoint = val
	b.markFieldAsDirty(currentJustifiedCheckpoint)
	return nil
}

// SetFinalizedCheckpoint for the beacon state.
func (b *BeaconState) SetFinalizedCheckpoint(val *ethpb.Checkpoint) error {
	if !b.HasInnerState() {
		return ErrNilInnerState
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.state.FinalizedCheckpoint = val
	b.markFieldAsDirty(finalizedCheckpoint)
	return nil
}

// Recomputes the branch up the index in the Merkle trie representation
// of the beacon state. This method performs map reads and the caller MUST
// hold the lock before calling this method.
func (b *BeaconState) recomputeRoot(idx int) {
	hashFunc := hashutil.CustomSHA256Hasher()
	layers := b.merkleLayers
	// The merkle tree structure looks as follows:
	// [[r1, r2, r3, r4], [parent1, parent2], [root]]
	// Using information about the index which changed, idx, we recompute
	// only its branch up the tree.
	currentIndex := idx
	root := b.merkleLayers[0][idx]
	for i := 0; i < len(layers)-1; i++ {
		isLeft := currentIndex%2 == 0
		neighborIdx := currentIndex ^ 1

		neighbor := make([]byte, 32)
		if layers[i] != nil && len(layers[i]) != 0 && neighborIdx < len(layers[i]) {
			neighbor = layers[i][neighborIdx]
		}
		if isLeft {
			parentHash := hashFunc(append(root, neighbor...))
			root = parentHash[:]
		} else {
			parentHash := hashFunc(append(neighbor, root...))
			root = parentHash[:]
		}
		parentIdx := currentIndex / 2
		// Update the cached layers at the parent index.
		layers[i+1][parentIdx] = root
		currentIndex = parentIdx
	}
	b.merkleLayers = layers
}

func (b *BeaconState) markFieldAsDirty(field fieldIndex) {
	_, ok := b.dirtyFields[field]
	if !ok {
		b.dirtyFields[field] = true
	}
	// do nothing if field already exists
}

// AddDirtyIndices adds the relevant dirty field indices, so that they
// can be recomputed.
func (b *BeaconState) AddDirtyIndices(index fieldIndex, indices []uint64) {
	b.dirtyIndices[index] = append(b.dirtyIndices[index], indices...)
}
