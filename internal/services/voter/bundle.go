package voter

import (
	"github.com/rarimo/near-go/common"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
)

func appendBundleIfExists(msg *oracletypes.MsgCreateTransferOp, event *common.BridgeEventData) *oracletypes.MsgCreateTransferOp {
	if event.BundleData != nil && len(*event.BundleData) > 0 && event.BundleSalt != nil && len(*event.BundleSalt) > 0 {
		msg.BundleData = *event.BundleData
		msg.BundleSalt = *event.BundleSalt
	}

	return msg
}
