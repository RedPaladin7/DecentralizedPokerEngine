package network

import (
	"context"
	"fmt"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/fault"
)

// TableShareHandNum is the v1 hand_num on KEY_SHARE. d does not rotate between
// hands, so shares are distributed once per table, not every hand.
const TableShareHandNum int64 = 0

// DesignatedSurvivor is the first remaining SeatOrder id (not in gone).
// gone may be map[string]struct{} or map[string]*SRAKey; any map with string keys works
// via the goneSet helper.
func DesignatedSurvivor(seatOrder []string, gone map[string]*pokercrypto.SRAKey) string {
	for _, id := range seatOrder {
		if _, ok := gone[id]; !ok {
			return id
		}
	}
	return ""
}

func DesignatedSurvivorIDs(seatOrder []string, gone map[string]struct{}) string {
	for _, id := range seatOrder {
		if _, ok := gone[id]; !ok {
			return id
		}
	}
	return ""
}

// DistributeLocalShares splits local d and unicasts one share to each other seat.
// The owner keeps their own share locally. Never put shares on the shuffle/peel bus.
// Call once after KeyringFromLobby; do not re-split on later hands.
func DistributeLocalShares(ctx context.Context, node *Node, fm *fault.FaultManager, kr *pokercrypto.Keyring, sraKey *pokercrypto.SRAKey) error {
	if node == nil || fm == nil || kr == nil || sraKey == nil {
		return fmt.Errorf("DistributeLocalShares: nil argument")
	}
	n := kr.Len()
	shares, thresh, err := fault.SplitAndDistribute(sraKey, n)
	if err != nil {
		return fmt.Errorf("DistributeLocalShares: %w", err)
	}
	fm.SetShamirThreshold(thresh)
	order := kr.SeatOrder()
	localID := kr.LocalID()
	if len(shares) != len(order) {
		return fmt.Errorf("DistributeLocalShares: share count %d != seats %d", len(shares), len(order))
	}
	for i, id := range order {
		if id == localID {
			fm.StoreKeyShare(localID, shares[i])
			continue
		}
		pid, err := PeerIDFromString(id)
		if err != nil {
			fmt.Printf("[crypto] key share to %s: %v\n", id, err)
			continue
		}
		var lastErr error
		for attempt := 0; attempt < 6; attempt++ {
			lastErr = node.SendDirectKeyShare(ctx, pid, TableShareHandNum, localID, shares[i])
			if lastErr == nil {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(400 * time.Millisecond):
			}
		}
		if lastErr != nil {
			fmt.Printf("[crypto] key share to %s: %v\n", id, lastErr)
		}
	}
	return nil
}
