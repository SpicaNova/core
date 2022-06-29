package app

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	antetypes "github.com/terra-money/core/v2/app/ante/types"
)

// UpgradeHandler h for software upgrade proposal
type UpgradeHandler struct {
	*TerraApp
}

// NewUpgradeHandler return new instance of UpgradeHandler
func NewUpgradeHandler(app *TerraApp) UpgradeHandler {
	return UpgradeHandler{app}
}

// CreateUpgradeHandler return upgrade handler for software upgrade proposal
func (h UpgradeHandler) CreateUpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
		// Minimum Commission Upgrade
		// check & update all validators commission
		if err := h.handleMinCommission(ctx); err != nil {
			return nil, err
		}

		if ctx.ChainID() == MainnetChainID {
			// Remove vesting token from TFL inter-chain account and Ozone wallets
			// and Send back to community pool
			if err := h.handleFundingCommunityPoolFromTFLOzoneWallets(ctx, []string{
				"terra1xtlkxkund5xxsj8uj94y6fmx2sf9unkc9l7lpg", // @zon
				"terra19epdm5jp8vdpm7mvfuflyzys4k0xk5vmgcv0xw", // TFL Finance
				"terra1zf8s0kq5uzcnm7zkmvjeqrlwfdapgfz007rpx0", // @JeremyDelphi
				"terra1dyn97p558vchcje0zycwfpex7xc67w4zl62nfy", // @lejimmy
				"terra1mrutxf7adxg837jl6z85g83pwsn9a2jh3xu9yy", // @Papi
				"terra16l79rtfr2pjcax50ptxs69zaxzntvg433mtqpc", // Chauncey from Angel
				"terra1zf8s0kq5uzcnm7zkmvjeqrlwfdapgfz007rpx0", // Jeremy from Delphi
				"terra1swnt7a6qxmht207ct4l36uetq38zm5nsgkyseu", // Remi from LFG
				"terra1w38qx5lppk3t57p99p56dln5cwaqxgt8rmxr0e", // Jonathan from Levana
				"terra19epdm5jp8vdpm7mvfuflyzys4k0xk5vmgcv0xw", // TFL
				"terra1gawu5a5gmxtfsrkjh034rfy5eclp49seuyxuz8", // Risk Harbor
				"terra1uckn8fmkx7yuv6asqx7azt2hpu0wnnpd4hvu0x", // Nick from Chronos
			}); err != nil {
				return nil, err
			}

			// Allocate community funds to the target addresses and apply vesting with staking
			// 70% will be vesting (2 year vesting with 6 month cliff)
			if err := h.tokenAllocateHandler(ctx, MainnetGenesisTime, map[string]int64{
				"terra1qapv4kngzdrhw3y2y08g0r9776ep5p645sdjyq": int64(1_112_664_830_000), // swissborg
				"terra10g8ln6ak9hdexje79k0dl5y82fl0g4er52djfh": int64(153_643_860_000),   // hex
				"terra1h5lvn89fp5pgs4gapzxy3zfqm6ye8wcfc7lhkf": int64(209_615_890_000),   // hex
				"terra186vxc9ywn2xmc03zu5zzracnxvshu4t3f0mlq2": int64(12_858_720_000),    // hex
			}); err != nil {
				return nil, err
			}

			// Update some exchange wallets vesting schedule to
			// => 30% unlock and 2 year vesting with 6 month cliff
			if err := h.vestingScheduleUpdateHandler(ctx, MainnetGenesisTime, map[string]int64{
				"terra1lgdpa7xl7n5k9wg65chldpkc06j2p5kg2cgf8w": int64(274_002_410_000),   // coinspot
				"terra16tjyr2qr3evaeucmvdl7w0kld65rthj40lsp0t": int64(3_881_280_000),     // coinspot
				"terra1u868n8kekvez2lnrz44ca00ufzk78rux3sn8m2": int64(5_174_730_000),     // bitkub
				"terra156jcf5xq0ureeu8ew7qetpur0v4v8sywsshg8p": int64(3_203_900_000),     // hitbtc
				"terra1ltt07sqsf42xhgfkwtpyp7ynahp5h67up6sdgd": int64(30_320_950_000),    // hex
				"terra1chq5ps8yya004gsw4xz62pd4psr5hafe7kdt6d": int64(1_136_894_400_000), // kucoin
			}); err != nil {
				return nil, err
			}
		}

		return h.mm.RunMigrations(ctx, h.configurator, vm)
	}
}

func (h UpgradeHandler) handleMinCommission(ctx sdk.Context) error {
	minimumCommission := antetypes.DefaultMinimumCommission
	allValidators := h.StakingKeeper.GetAllValidators(ctx)
	for _, validator := range allValidators {
		// increase commission rate
		if validator.Commission.CommissionRates.Rate.LT(minimumCommission) {

			// call the before-modification hook since we're about to update the commission
			h.StakingKeeper.BeforeValidatorModified(ctx, validator.GetOperator())

			validator.Commission.Rate = minimumCommission
			validator.Commission.UpdateTime = ctx.BlockHeader().Time
		}

		// increase max commission rate
		if validator.Commission.CommissionRates.MaxRate.LT(minimumCommission) {
			validator.Commission.MaxRate = minimumCommission
		}

		h.StakingKeeper.SetValidator(ctx, validator)
	}

	return nil
}

func (h UpgradeHandler) handleFundingCommunityPoolFromTFLOzoneWallets(ctx sdk.Context, addresses []string) error {
	bondDenom := h.StakingKeeper.BondDenom(ctx)
	for _, addr := range addresses {
		accAddr, err := sdk.AccAddressFromBech32(addr)
		if err != nil {
			return err
		}

		// If account not exist, then skip the process
		account := h.AccountKeeper.GetAccount(ctx, accAddr)
		if account == nil {
			continue
		}

		// The current spendable coins are personal coins,
		// so should be left after upgrade
		spendableCoins := h.BankKeeper.SpendableCoins(ctx, accAddr)

		// Unbond all delegation without lock period
		// this will withdraw all staking rewards
		unbondedAmountFromBondedPool := sdk.ZeroInt()
		unbondedAmountFromNotBondedPool := sdk.ZeroInt()
		delegations := h.StakingKeeper.GetAllDelegatorDelegations(ctx, accAddr)
		for _, del := range delegations {
			amount, err := h.StakingKeeper.Unbond(ctx, accAddr, del.GetValidatorAddr(), del.GetShares())
			if err != nil {
				return err
			}

			if amount.IsPositive() {
				if h.StakingKeeper.Validator(ctx, del.GetValidatorAddr()).IsBonded() {
					unbondedAmountFromBondedPool = unbondedAmountFromBondedPool.Add(amount)
				} else {
					unbondedAmountFromNotBondedPool = unbondedAmountFromNotBondedPool.Add(amount)
				}
			}
		}

		// Finish unbonding delegations
		unbondingDelegations := h.StakingKeeper.GetAllUnbondingDelegations(ctx, accAddr)
		for _, unbonding := range unbondingDelegations {
			for i, entry := range unbonding.Entries {
				unbondedAmountFromNotBondedPool = unbondedAmountFromNotBondedPool.Add(entry.Balance)
				unbonding.RemoveEntry(int64(i))
			}
		}

		if unbondedAmountFromBondedPool.IsPositive() {
			unbondedCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, unbondedAmountFromBondedPool))
			if err := h.BankKeeper.UndelegateCoinsFromModuleToAccount(
				ctx, stakingtypes.BondedPoolName, accAddr, unbondedCoins); err != nil {
				return err
			}
		}

		if unbondedAmountFromNotBondedPool.IsPositive() {
			unbondedCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, unbondedAmountFromNotBondedPool))
			if err := h.BankKeeper.UndelegateCoinsFromModuleToAccount(
				ctx, stakingtypes.NotBondedPoolName, accAddr, unbondedCoins); err != nil {
				return err
			}
		}

		// Convert vesting account to normal account
		h.AccountKeeper.SetAccount(ctx, authtypes.NewBaseAccount(
			account.GetAddress(),
			account.GetPubKey(),
			account.GetAccountNumber(),
			account.GetSequence(),
		))

		// Fund Community Pool
		allCoins := h.BankKeeper.GetAllBalances(ctx, accAddr)
		if err := h.DistrKeeper.FundCommunityPool(ctx, allCoins.Sub(spendableCoins), accAddr); err != nil {
			return err
		}
	}

	return nil
}

func (h UpgradeHandler) tokenAllocateHandler(ctx sdk.Context, genesisTime int64, allocationMap map[string]int64) error {
	bondDenom := h.StakingKeeper.BondDenom(ctx)

	for addr, amount := range allocationMap {
		allocatedAmount := sdk.NewInt(amount)
		accAddr, err := sdk.AccAddressFromBech32(addr)
		if err != nil {
			return err
		}

		// Allocate token to the target recipient from the community pool
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, allocatedAmount))
		h.DistrKeeper.DistributeFromFeePool(ctx, coins, accAddr)

		// Update vesting schedule
		// 70% are newly added as vesting
		vestingAmount := sdk.NewDecWithPrec(7, 1).MulInt(allocatedAmount).TruncateInt()

		account := h.AccountKeeper.GetAccount(ctx, accAddr)
		vestingAccount := account.(*vestingtypes.PeriodicVestingAccount)

		// Increase OriginalVesting
		vestingAccount.OriginalVesting = vestingAccount.OriginalVesting.Add(sdk.NewCoin(bondDenom, vestingAmount))

		// 2 year vesting with 6 month cliff
		vestingAccount.StartTime = genesisTime + 60*60*24*30*6
		vestingAccount.VestingPeriods = vestingtypes.Periods{
			{
				Length: 60 * 60 * 24 * 365 * 2,
				Amount: vestingAccount.OriginalVesting,
			},
		}

		// Track delegation
		// all original vesting tokens are still in vesting, so use
		// OriginalVesting instead of VestingCoins
		delegatedAmount := vestingAccount.DelegatedFree.Add(vestingAccount.DelegatedVesting...)
		if vestingAccount.OriginalVesting.IsAllGTE(delegatedAmount) {
			vestingAccount.DelegatedVesting = delegatedAmount
			vestingAccount.DelegatedFree = sdk.NewCoins()
		} else {
			vestingAccount.DelegatedVesting = vestingAccount.OriginalVesting
			vestingAccount.DelegatedFree = delegatedAmount.Sub(vestingAccount.OriginalVesting)
		}

		// update account
		h.AccountKeeper.SetAccount(ctx, vestingAccount)
	}

	return nil
}

func (h UpgradeHandler) vestingScheduleUpdateHandler(ctx sdk.Context, genesisTime int64, unlockAmountMap map[string]int64) error {
	bondDenom := h.StakingKeeper.BondDenom(ctx)
	for address, unlockAmount := range unlockAmountMap {
		accAddr, err := sdk.AccAddressFromBech32(address)
		if err != nil {
			return err
		}

		// Unlock amount are already allocated at genesis as vesting
		// but only vesting schedules are not properly set.
		// Need to decrease OriginalVesting to unlock the tokens
		account := h.AccountKeeper.GetAccount(ctx, accAddr)
		vestingAccount := account.(*vestingtypes.PeriodicVestingAccount)
		vestingAccount.OriginalVesting = vestingAccount.OriginalVesting.Sub(
			sdk.NewCoins(sdk.NewCoin(bondDenom, sdk.NewInt(unlockAmount))),
		)

		// Track delegation - decrease delegated vesting
		// and increase delegated free amount
		originalVesting := vestingAccount.OriginalVesting.AmountOf(bondDenom)
		delegatedVesting := vestingAccount.DelegatedVesting.AmountOf(bondDenom)
		delegatedFree := vestingAccount.DelegatedFree.AmountOf(bondDenom)
		delegatedAmount := delegatedFree.Add(delegatedVesting)
		if delegatedVesting.GT(originalVesting) {
			vestingAccount.DelegatedVesting = sdk.NewCoins(sdk.NewCoin(bondDenom, originalVesting))
			vestingAccount.DelegatedFree = sdk.NewCoins(sdk.NewCoin(bondDenom, delegatedAmount.Sub(originalVesting)))
		}

		// 2 year vesting with 6 month cliff
		vestingAccount.StartTime = genesisTime + 60*60*24*30*6
		vestingAccount.VestingPeriods = vestingtypes.Periods{
			{
				Length: 60 * 60 * 24 * 365 * 2,
				Amount: vestingAccount.OriginalVesting,
			},
		}
	}

	return nil
}
