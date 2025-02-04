package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/terra-money/core/v2/app/ante/types"
)

// MinimumCommissionEnforced - the flag represents whether minimum commission enforced or not
func (k Keeper) MinimumCommissionEnforced(ctx sdk.Context) (res bool) {
	k.paramstore.Get(ctx, types.ParamStoreKeyMinimumCommissionEnforced, &res)
	return
}

// Get all parameteras as types.Params
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	return types.NewParams(
		k.MinimumCommissionEnforced(ctx),
	)
}

// SetParams set the params
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) {
	k.paramstore.SetParamSet(ctx, &params)
}
