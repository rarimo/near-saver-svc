package voter

import (
	"encoding/json"
	"github.com/rarimo/near-go/common"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"strings"
)

var eventPrefix = "EVENT_JSON:"

var ErrEventNotFound = errors.New("event not found")

func extractEvent(tx *common.FinalExecutionOutcomeWithReceiptView, contractAddr, eventID string) (*common.BridgeEvent, error) {
	var event *common.BridgeEvent

	for _, receiptOutcome := range tx.FinalExecutionOutcomeView.ReceiptsOutcome {
		// Skipping receipt if it's not related to the contract
		if receiptOutcome.Outcome.ExecutorID != contractAddr {
			continue
		}

		// Skipping receipt if it's not related to the event
		if receiptOutcome.ID.String() != eventID {
			continue
		}

		for _, log := range receiptOutcome.Outcome.Logs {
			event = GetEventFromLog(log)
			if event == nil {
				continue
			}
		}
	}

	if event == nil {
		return nil, ErrEventNotFound
	}

	return event, nil
}

func GetEventFromLog(log string) *common.BridgeEvent {
	if !strings.HasPrefix(log, eventPrefix) {
		return nil
	}

	eventRaw := strings.TrimPrefix(log, eventPrefix)
	var event common.BridgeEvent

	err := json.Unmarshal([]byte(eventRaw), &event)
	if err != nil {
		// Skipping event if it's not valid NEP-297 event https://nomicon.io/Standards/EventsFormat
		return nil
	}

	if !validateDepositEvent(event) {
		return nil
	}

	return &event
}

func validateDepositEvent(event common.BridgeEvent) bool {
	switch common.BridgeEventType(event.Event) {
	case common.NFTEventType, common.FTEventType, common.NativeEventType:
		return true
	default:
		return false
	}
}
