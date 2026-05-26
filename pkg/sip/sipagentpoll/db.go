package sipagentpoll

import (
	"context"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/internal/models"
	"gorm.io/gorm"
)

var (
	dbMu sync.RWMutex
	db   *gorm.DB
)

// SetDatabase wires GORM for persisting transfer-offer rows (call from cmd/sip after DB init).
func SetDatabase(gdb *gorm.DB) {
	dbMu.Lock()
	db = gdb
	dbMu.Unlock()
}

func getDB() *gorm.DB {
	dbMu.RLock()
	defer dbMu.RUnlock()
	return db
}

func persistStartRinging(acdTargetID uint, inboundCallID, callerNumber string) {
	gdb := getDB()
	if gdb == nil {
		return
	}
	_, _ = models.StartSIPACDTransferOffer(context.Background(), gdb, acdTargetID, inboundCallID, callerNumber)
}

func persistFinishInbound(inboundCallID, phase string) {
	gdb := getDB()
	if gdb == nil {
		return
	}
	_ = models.FinishSIPACDTransferOffersByInbound(context.Background(), gdb, inboundCallID, phase)
}

func persistFinishACD(acdTargetID uint, phase string) {
	gdb := getDB()
	if gdb == nil {
		return
	}
	_ = models.FinishSIPACDTransferOffersByACD(context.Background(), gdb, acdTargetID, phase)
}

func activeOfferFromDBFresh(acdTargetID uint) (Snapshot, bool) {
	gdb := getDB()
	if gdb == nil || acdTargetID == 0 {
		return Snapshot{}, false
	}
	row, ok, err := models.ActiveSIPACDTransferOffer(context.Background(), gdb, acdTargetID)
	if err != nil || !ok {
		return Snapshot{}, false
	}
	age := time.Since(row.StartedAt.UTC())
	if age > maxDBRingingAge {
		_ = models.FinishSIPACDTransferOffersByInbound(context.Background(), gdb, row.InboundCallID, models.SIPACDTransferOfferPhaseCancelled)
		return Snapshot{}, false
	}
	return Snapshot{
		Incoming:      true,
		InboundCallID: row.InboundCallID,
		CallerNumber:  row.CallerNumber,
		Phase:         row.Phase,
		ACDTargetID:   acdTargetID,
		Since:         row.StartedAt.UTC().Format(time.RFC3339),
	}, true
}
