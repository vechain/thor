// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"math/big"

	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// NewMainnet create mainnet genesis.
func NewMainnet() *Genesis {
	launchTime := uint64(1530316800) // '2018-06-30T00:00:00.000Z'

	initialAuthorityNodes := loadAuthorityNodes()

	builder := new(Builder).
		Timestamp(launchTime).
		GasLimit(thor.InitialGasLimit).
		ForkConfig(thor.NoFork).
		State(func(state *state.State) error {
			// alloc builtin contracts
			if err := state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Executor.Address, builtin.Executor.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}

			tokenSupply := &big.Int{}
			energySupply := &big.Int{}

			// alloc tokens for authority node endorsor
			for _, anode := range initialAuthorityNodes {
				tokenSupply.Add(tokenSupply, thor.InitialProposerEndorsement)
				if err := state.SetBalance(anode.endorsorAddress, thor.InitialProposerEndorsement); err != nil {
					return err
				}
				if err := state.SetEnergy(anode.endorsorAddress, &big.Int{}, launchTime); err != nil {
					return err
				}
			}

			// alloc all other tokens
			// 21,046,908,616.5 x 4
			amount := new(big.Int).Mul(big.NewInt(210469086165), big.NewInt(1e17))
			tokenSupply.Add(tokenSupply, amount)
			if err := state.SetBalance(thor.MustParseAddress("0x137053dfbe6c0a43f915ad2efefefdcc2708e975"), amount); err != nil {
				return err
			}
			if err := state.SetEnergy(thor.MustParseAddress("0x137053dfbe6c0a43f915ad2efefefdcc2708e975"), &big.Int{}, launchTime); err != nil {
				return err
			}

			tokenSupply.Add(tokenSupply, amount)
			if err := state.SetBalance(thor.MustParseAddress("0xaf111431c1284a5e16d2eecd2daed133ce96820e"), amount); err != nil {
				return err
			}
			if err := state.SetEnergy(thor.MustParseAddress("0xaf111431c1284a5e16d2eecd2daed133ce96820e"), &big.Int{}, launchTime); err != nil {
				return err
			}

			tokenSupply.Add(tokenSupply, amount)
			if err := state.SetBalance(thor.MustParseAddress("0x997522a4274336f4b86af4a6ed9e45aedcc6d360"), amount); err != nil {
				return err
			}
			if err := state.SetEnergy(thor.MustParseAddress("0x997522a4274336f4b86af4a6ed9e45aedcc6d360"), &big.Int{}, launchTime); err != nil {
				return err
			}

			tokenSupply.Add(tokenSupply, amount)
			if err := state.SetBalance(thor.MustParseAddress("0x0bd7b06debd1522e75e4b91ff598f107fd826c8a"), amount); err != nil {
				return err
			}
			if err := state.SetEnergy(thor.MustParseAddress("0x0bd7b06debd1522e75e4b91ff598f107fd826c8a"), &big.Int{}, launchTime); err != nil {
				return err
			}

			return builtin.Energy.Native(state, launchTime).SetInitialSupply(tokenSupply, energySupply)
		})

	///// initialize builtin contracts

	// initialize params
	data := mustEncodeInput(builtin.Params.ABI, "set", thor.KeyExecutorAddress, new(big.Int).SetBytes(builtin.Executor.Address[:]))
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, thor.InitialRewardRatio)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), builtin.Executor.Address)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyBaseGasPrice, thor.InitialBaseGasPrice)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), builtin.Executor.Address)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), builtin.Executor.Address)

	// add initial authority nodes
	for _, anode := range initialAuthorityNodes {
		data := mustEncodeInput(builtin.Authority.ABI, "add", anode.masterAddress, anode.endorsorAddress, anode.identity)
		builder.Call(tx.NewClause(&builtin.Authority.Address).WithData(data), builtin.Executor.Address)
	}

	// add initial approvers (steering committee)
	for _, approver := range loadApprovers() {
		data := mustEncodeInput(builtin.Executor.ABI, "addApprover", approver.address, thor.BytesToBytes32([]byte(approver.identity)))
		builder.Call(tx.NewClause(&builtin.Executor.Address).WithData(data), builtin.Executor.Address)
	}

	var extra [28]byte
	copy(extra[:], "Salute & Respect, Ethereum!")
	builder.ExtraData(extra)
	id, err := builder.ComputeID()
	if err != nil {
		panic(err)
	}
	return &Genesis{builder, id, "mainnet"}
}

type authorityNode struct {
	masterAddress   thor.Address
	endorsorAddress thor.Address
	identity        thor.Bytes32
}

type approver struct {
	address  thor.Address
	identity string
}

func loadApprovers() []*approver {
	return []*approver{
		{thor.MustParseAddress("0xb0f6d9933c1c2f4d891ca479343921f2d32e0fad"), "CY Cheung"},
		{thor.MustParseAddress("0xda48cc4d23b41158e1294e0e4bcce8e9953cee26"), "George Kang"},
		{thor.MustParseAddress("0xca7b45abe0d421e5628d2224bfe8fa6a6cf7c51b"), "Jay Zhang"},
		{thor.MustParseAddress("0xa03f185f2a0def1efdd687ef3b96e404869d93de"), "Margaret Rui Zhu"},
		{thor.MustParseAddress("0x74bac19f78369637db63f7496ecb5f88cc183672"), "Peter Zhou"},
		{thor.MustParseAddress("0x5fefc7836af047c949d1fea72839823d2f06f7e3"), "Renato Grottola"},
		{thor.MustParseAddress("0x7519874d0f7d31b5f0fd6f0429a4e5ece6f3fd49"), "Sunny Lu"},
	}
}

func loadAuthorityNodes() []*authorityNode {
	all := [...][3]string{
		{"0xdbe84597403b9aec770aef4a93a3065b3b58d306", "0xb3a4831cadcee1efb78028c2ba72f29f22a197e1", "0xb11c5752af4c9ab07e7379e693e47ffba97e1f4f686128cea601b2ea64646732"},
		{"0x0625e7b8a7c2e696cb31fd130162189314520171", "0x9366662519dc456bd5b8bc4ee4b6852338d82f08", "0x0ff7f5023e49ab9f558d7d9a743fb3a864ebde1f2497b896b527f970a292d7da"},
		{"0x87f639b87ecb6db2b202f93b66fdb88510b96689", "0xce42d8faf4694840eb54ac0006c59d3024f64b75", "0x99d558729d67bd21e2a945d4064bc6111e6c9162d41cd0c3384615fb87189e69"},
		{"0x61bf7225217f0c26a4b99b6a534eb926bcb943be", "0xc83e49f3abf2ce3794b66d14adc2176bad1be6e7", "0xb2c6a45e5ea14249ca90556f723fa130d9a1ee3eebdaf6f961185cbb50cd4933"},
		{"0x6d9a1d56090ca3f0258e23b76c339aaf2e2a6cb5", "0xf42d0a0df8cfb2a9269511f1a6ba0e9e38c4b497", "0x82360f5a24b3a1693aaa38f470f4f47fd99b572b1d71180e3a667a6572ca722d"},
		{"0x8ee3b768b460d9a199e2c19eda7935f37b4a7b6e", "0xd6d918cb7870c5752fe033c3461db32bcdb64fbd", "0xa210053efa867b563ca507dfe1789ccb0d4a1cf5da13d13a792cd92c7a422092"},
		{"0x76b3c95ce7a92164f48d7f909b8cee21156fed6e", "0x1978fd27183aae3116dde718df7a6372bd6f8a9a", "0xc80b56695f0b540489d2c5aa4449ac538d0066cb2ad1b12866680939d117f908"},
		{"0xc25f1d1f0beb16176c87c7181644987a2528da79", "0xb447ab2851f9a485cfbb21305c4896b45f9bc0dd", "0x481a09465a276311a061cef846fb2201e7735c78d185f3400fb3aa0f14255685"},
		{"0x1c5e5cae556c86ef87e07a3266f152dba86a370d", "0x21d38fdf726c138ae73c78075159a4ece3130ee2", "0x5d40002c8a9e52005897baea9e9753bd7dffd00e0976f614e34f5a5aedf15061"},
		{"0x5cc7e442249e0508880a3489155cd9e45aeae2e9", "0xe20a2347081bb1328532978fe896ee13c536d7ba", "0xd2f9f12a8287680721b227baf4cb232a527799b0ab55a22289fe4cb96122f5c0"},
		{"0xfc8e7994bb302ad21f19e3418221a66e65e8cfcc", "0xda5f12f45b42a207e58b8d430fcde11c0b54e68d", "0x9210db81725c3bda7983e6fadd5ed8accc2951b5dca7414fd25886e84a517546"},
		{"0xf03307f9b4155ecd1e8be96c74a4cf3676d93c76", "0x986501e98957f36c933ca419f060281c9c35dc41", "0xc068df75ea550ff6a0dda67c43de67c0176f5cd5c2db56d0436ca656b2f64f24"},
		{"0x359ac7f5f025334dc08ece27b4cc4f8a0c06b52d", "0x057bc0107d9039b3cc346958ed38e961032d3dc6", "0xa51d6942562f54c2f241642d4526a4d6c0137e14b2071dc6cf3fd464be3c7fa2"},
		{"0xd42a174bbeac8695d4c379750ba71bfcca8ef60e", "0x1a6cd62f72315b926e7310e330d84b26db32dd85", "0xa2f45e4058b2d6ebeea7ef158169691b1921e031ddb0d45dca589bfd46153107"},
		{"0x6935455ef590eb8746f5230981d09d3552398018", "0xb5358b034647202d0cd3d1bf615e63e498e02682", "0x49984a53f9397370079bba8d95f5c15c743098fb318483e0cb6bbf46ec89ccfb"},
		{"0xfbda2b695a3406b9746d38d78322b926e416e687", "0xe03388a87f0d2e56701048b3b41a4ca0ab068da7", "0x834d6a4cac5000a951b22a98ee9da9927170b0c7dacc8135e849db2fd9a27496"},
		{"0xeb921887026118e79a348c5fd2cdec33a2116423", "0xa906fe50b3a807a7ab0205051ac3a1f2211af613", "0x3a775b076d56e530fb02f733090d0bec7454a2904c2347de1e9090196e17faa1"},
		{"0x67c9911117a4ac4a12fe62a8847bb8a6ceae3dca", "0x0870089330741c126d92bf5759011bb31e24873f", "0x822b0b44b6d6bc04510422cb3ebd6a63312ef09b6d7a1f9ac415dbc9c1628ceb"},
		{"0x70aabde7f52c65e107a92d720024ff17543b6e36", "0x4fc5342543e6d2dc34fc410d266692bc45c483f9", "0x3417ba7f1c47f90e920229d612544b2c4b78147e0d11016f0a805cdcd7c09dd2"},
		{"0x2ed9f63e3b589e76617e6e80853f187ee764ffcd", "0x77efe1bb436ca9d6537219681ae76e9de2c79ef5", "0xe4820e922587d6bfe5bec6375eccbfd420b2dda932f818ae512f0e1841ca4616"},
		{"0x8eaefdf7d25c001e7e59363c33d7f5ad47970086", "0xeb0c565f69557481c6c7fa347cae273128a0996e", "0x3566cd2b6343b02972383d6aaaefb602ac8c68a7d5f982e2e8937c1db3b1beee"},
		{"0x473b001dc2bc56eca9020453b4365fa6389cf625", "0xc8be0902a99f4acf0fcfa2e2462eb3c6774725d6", "0xed25ebdb23b67ddcaba3eba42a435491d2103e2a5517178d31f9f9ceaabb22aa"},
		{"0x552761f4e9520653c25f841acce7de5cf1144421", "0x049db80366ec1509d96acef90e7974a66c7fe0ae", "0x079860bdcd90e13fcf2a9aa635343947f6523352fd16929306f30bf1280069c7"},
		{"0xde318dd878acada99a480e2364e6352d3b3bf737", "0x80737961e5fee5ee7ece81564d66e47179a02a84", "0xb21db1b49f6fe03c226da11f9836fd31749ecad6db6e80b35540820f2c7b642a"},
		{"0xed563e0f2da140c65c1ed55bd1966188a8e70f0e", "0x63559f24a8f38cf1ed2f6f5ba67c6fdd08432cd1", "0x6bb6d1afce281ad9d7f1a2074071267cf43142fecabde8b6b6d3fdf5a0f42021"},
		{"0xda1991a40780cad1123e7f23a4eea0fcd20ca2ba", "0xa78994bbeddc697dd540de178a6ee66c240374a0", "0x9626e00f80d25ac8716f59073da570e9204b817f384e2f08abba64c60c8e60d4"},
		{"0x5ff66ee3a3ea2aba2857ea8276edb6190d9a1661", "0xd51666c6b4fed6070a78691f1f3c8e79ad02e3a0", "0x76f090d383f49d8faab2eb151241528a552f0ae645f460360a7635b8883987a6"},
		{"0xf91154568911f757642a4594f7561fec77b62e8a", "0x7dfdcd8a4559c285a3af242472d9d11289a8e46e", "0x1e0c826327d074bce22ebd0b94120acb2a18fd4b635c94764f7e7e061ea0b5f2"},
		{"0x11bf1a999f135c5a92ceacba9c5fc035b08fc631", "0x94afc0c08ee7d3cd1540a51eece705dbadc3dedc", "0x00922d419c405d29039c5eadf3089556d7b3c66526376926bd532c3f75b6fe53"},
		{"0x75b7746324faf7b559e378a3a88012db3cc796db", "0x224626926a7a12225a60e127cec119c939db4a5c", "0xdbf2712e19af00dc4d376728f7cb06cc215c8e7c53b94cb47cefb4a26ada2a6c"},
		{"0xc5a02c1eac7516a9275d86c1cb39a5262b8684a4", "0xe32499b4143830f2526c79d388ecee530b6357aa", "0xc635894a50ce5c74c62d238dbe95bd6a0fa076029d913d76b0d0b111c538153f"},
		{"0x370bd316715055c5aceaaab5f1078fe0813064c3", "0x9d3002f06bf33a5d2cf0839c0298739963e48bbf", "0x7db78a789305cd079654ea2b398c517839b3534af17d49922481554c4693f6b0"},
		{"0x66b6691952b1897b41fa0b90a87202a7402aaba8", "0x987b68e1b71d87b82ffce7539ae95b1b11ac7eb0", "0x84fa22e3ffa25136c064f2448eabf0e7a3070206f0c8fd02e0cf07f1d126d122"},
		{"0x21d38922853e78adca05f43507759df6dae89702", "0x15dcee2cde4fafad607c4f3e9629dd94486e14d0", "0x27a8b7cdbdd73905f269ed9a659d229c02fed0e0cf73af5c972b256f4be7104e"},
		{"0xdb71c5c9a5cd407313139978aa8990347d141918", "0x14b18a1cd33e43eb72aaf1b8c65e2f9f067c4176", "0x67894478411bdbd33dd932217d2a4f8f3eec0892a301ce0fc584533342805c5a"},
		{"0x8b7cea9c7d73676d689043a51f0ad8123dd20928", "0x2034e870f70627a54dcdd6ace147feb046ec061d", "0xc9b3d57e64a17b704b3174f3e696713793ad04e085da7c3437aa363774a7217c"},
		{"0x6496038fff3df815ff8f8e4e7d02f1853aa86752", "0xd11ec91d6a52783d19641e21dd1d0b4060e58754", "0x8f33cbafa957d45a568df93e2a533362682fbaff50e38059812a614bd0947300"},
		{"0xb649678dcfd53a844c79da7a2bc6a884c47409c3", "0x0a10597f29733bedfad7520a3a7031b97368de11", "0x9d413e58bc463e10ab9b711c171e150fab68947e3b5c40c111bb6a7cc3bebc30"},
		{"0xebdb73e7612998b64ec1f74de1284c8a1597e1b0", "0xf691f4024582203634388fd5fc513c9cbc897942", "0x072dffffb4dd0f717f96774a173cc4495beb8aaa725f5bd9978f9643643e9b44"},
		{"0xedb958f30b52dac5dd600826ea0736be9d810beb", "0x69e3776ffebfabe68b59ce5797a4832ec2f89a19", "0x0e2e1500be0f615d48a9eb9228e360da36269126e86596a1e53e92e842f57331"},
		{"0x972433c971759f5c5cc1e6f482c827d9c60203b2", "0xe3fecc11358e51d4ddc317e631c0f5e648dde0fd", "0xc43a08840bb8e82b80240995b848bf91aff6657ccd5224523556e9a366b03e93"},
		{"0xeb9b6940c6adfa25b8783b1c3a55a71c80d9ec06", "0x11e698a23aa16df7485638b76be943b27371b921", "0x362c070010f74f2c36c2e42fe79390929bc705ac67dba8458a46352189f97331"},
		{"0xa62cd77c6a708c6628da67b377911e723ad7dc67", "0x9a1e4bf6c41f50c399a128ab588fe4e4883bd872", "0x1ab077dc0b090f36221f5b423b6f923a9ffd8721dd409b00b60dd7756e95c276"},
		{"0x40d53e87565d6db7a989a54112fa03dd82124faa", "0x2c59c15af29dcfe4d4601af3f50f943bd215f62d", "0xb620c68b1571a2255d45add8bb35b6a819eda3f8bf59e9d21368400c333450f3"},
		{"0x83a62f9edeb4607c4e883ab3483c9022b8f3e4fd", "0xb57ff89e8427ea5c477bed2083970a4493862194", "0xf9b690213e5a396a0bad90884eb9de9e4c114e693fd7073c26c8d323c048662b"},
		{"0xa9263fa1ff9f20b79166c79f9b2c789e55117482", "0xadf25ed7814c8b978f2cfd1de663d8f6b84145a0", "0x7fdcbaa07a0d1a6e27c6b8a6284066c38edf6f67f6f325c936ae8916da9e661e"},
		{"0xf8ec521765fe663efc60e4d84b8b2fb189287945", "0xaad9fa35d309e5f6da821376634febe0db201e51", "0x7f4861949940972ea8aef246a6982fb1d3bba968f0771dd7f5cffa8739d1a9e1"},
		{"0xccb44de6fd6a916f968cd6097537bb2b3b51ac73", "0x77bb58f46440c51e0990b9c0a28bae3e24fe25d9", "0xab459c826bbd37e429fe0202581b817c4cdeba7174fe69e07bc8148af9d57de3"},
		{"0x9d11a303ad8c404929620fcb7b23e091ff04c843", "0x0acc7bfc8f7d904fe4a35f32f529217c7ca75377", "0x88045c474ee55440f622cbff73b4bea6c9763f3966fd630cf2e0243f90d88214"},
		{"0x2da258cae01aac5cd0e4bd876f708081f78b327d", "0x5643143716537c9c86c558091d2a30710f71fec7", "0x7ea36d4c8420c8e6b9019834d8f4d88461bae44c9ecde4d0b37ca9e84a96f942"},
		{"0xbff662e248d81f3cb79188ecd521139a6e444f81", "0x190b8d3d0fc0946ea58b7ae52c9ea77338dd5613", "0x0f25b9ff484e655cf1538031c34a669111531498be33bf0030e8aa8b202289c8"},
		{"0xa456ace87af4154c7cb6085d97e958a3d9ccabb9", "0x555f26a336e36959d07bc58dab13e562a3b3a200", "0x001373baeb614fc9fb08a8a1ebe33f80f26350951a8cab69b208f67717521fa3"},
		{"0xb089b15a00528eeb19fca4565df80d9a111bfcf9", "0xa9e5617e2f90427f6db70a3b1d08fc14706eb907", "0xfeb9e53e115705b3287d1ecb9f2b969b29063b2124b29fd821d1425d36bcb183"},
		{"0xbbf0e627b9bd36a956127ea0b74e39dc8b8e96b8", "0xf17e6c22f9eedfc8fd2f731da9b9154b87354764", "0x6d00833b0204f96f0daae49c67333e1660b42315113eb3aefd82473c77a39264"},
		{"0x538603b6f259724d2a17ee9f4419826ad2e1ef81", "0xd5efb9c70c006bcf92b9ce60cda27a282229011d", "0xe5a716ee487f115107a66c92818f61cb36e574636b202df90c4b12589bb42ec3"},
		{"0x401d7b90711aef130e70cfcc1cf8b1800522e6c3", "0x807f7a34045eec8796cf9c1fca049348544361a9", "0x0b5e88f853a6219a4a89f1e2288b09e6d97cf90cb92d33fd9e7ebd39d2e0f2e0"},
		{"0x55fbd86a0316c9d0447c292763a66a89a504257b", "0xe0988034941c87fc18d3eae188a08955f8779cce", "0x856cb9c2303192d2e997ef38262f74d8b3940a92fab198d30ae995aad62fbc3c"},
		{"0xe23a716f858fa2dc236c5652de019bf4bbfe5740", "0x2265d467ec73fdd2f1e4f513d05c09b910707823", "0xb3afb33a4bad1625c04dc1c9a6a88ffe470f030aeda22371d543557102fdb26a"},
		{"0x3afb3ad080c4156ac8ee118ff20098a8e61036ad", "0x7dae11b4b67d0012bab625ab5cf0894e35b713bf", "0x5f463055513e6a9aac3e3832d24cf70d62b4e3a81c64dbdf32ae88c011b8e38d"},
		{"0x6d8617ccf11b76cfb4c73e367fb5fe0c026c6b66", "0xe446ad66616fb97659c84d868357b99c0418836f", "0xbaa6d4dc2677d140850b64a4b8d0c91df562025af2392d41b44d3066e64c7193"},
		{"0xc3b3770ecabd9410f70b27612da8242eddd70dcf", "0x54f6f89138b7ff8131fa76485d1a81cc1e8fe2b7", "0x44924fbca6ca3e07295af34e34158f32e1061af67392ee92563b5e0a71b31bab"},
		{"0x95abbad5ddea8f40de15e04473c719f45c24d953", "0xa1cba33c939a5f8956b760970be3c06666c9103d", "0xbd502169c453dd8cc4346e2acdba6b3f3b0ba6977f4f089152ee7e81e806a7b1"},
		{"0x68fff2dc8217d5f722a5a2df9400cde367c9366e", "0x352f2753c668d9c970fc0f7bf54c0fce628b97a4", "0x2d4f7224310f4aa6e237d19964f4ef57fe56261fc39fcdaa1ab074c2a5406985"},
		{"0xc3c828f0db6add78beca8fd61017c368aa841253", "0x319c810685881d61910df826ff4982841f459a26", "0xab59ade89ff1a205ca53c1596ab45d1745342937695a85b661e31571e76368bd"},
		{"0x55c7e021c80284a87e3d71f403954d389264b5d6", "0xed4bb97cde61db2c397772d760a46679fd5fe92d", "0xdee1e01a06c52bac9a00d1667904bfe3754876f2c45ac22ff80ca48a484b0ad2"},
		{"0xe5e8563f66dc334f9399a0d1a675eb4708f2217b", "0x2c593298dc0913e2bb43b383be74132dd3d98e0c", "0x0fb01f072aeafab7bb0de2bd837ead5640250ab900a61b3657605301bc4398c3"},
		{"0x80934f7faaf80cb472ad275602f2e5c7c64b34e9", "0xb72f547b16c8ad64f28d698c7af5b38c8a1166f0", "0xe95687676b28d95602dbf6b68505040fa1a4e02cb60c8ae15dcdd483922d0a0a"},
		{"0x97f8bacc0c0284e17a1880dbeab1aea586ab57af", "0x0da8fa475c8272d21be204fe8112d1e2cd698c96", "0x4b65deb402c84cfce2c4f2f9568a6b6e24968ddb631dec20742cf739d87745c2"},
		{"0x16876b9aa6846c89cca49281c11fdf68d3543596", "0x65c620071eb78f201d4fe41a191ebd031809fca2", "0xb05246d155a23d12ce557b62ca1c5c60c6c3904e0c0076652c00d6efbf1aec4b"},
		{"0x120eae1a408183afcac4cbe5579d88535bdfbb4d", "0x7db64d97c157e863a49004e582fa66d0d6129ca5", "0x8ec51e708abebeae3f1a3c16d4778344209be3c392eadcbd1eeb41266e16a16a"},
		{"0xa3d1a8b393410cccb5e9dc51717a07a85e8d210e", "0xc3ed88082a22c41d31ab2c7484ff4530a7bd945e", "0x4a08fa7c78dc34e0a7dc9a37f22ce78a0d155015e4d80112be772c1f1221e43f"},
		{"0xa491eeb3da41cc957d4c700d4bd16c8aaf22637d", "0x62131a2c1181e8c38185b59236a8640b4c11786c", "0xfcc272c51ac2a7ade84a5dce72b3027b610738b9f98d39770ecc25478f820a27"},
		{"0x7831eed39ae7cd246cc539afafe8dddade0cc667", "0x46b3f22f3de9af93f5d46a2cfcc674acb4785f2f", "0x21db3f1e1cc1e21fd74cc52ca4c4539bd43d251174ead3109afe93195c58da6b"},
		{"0xf78dad3d05f2c54a7f4f42bc50e692d02a8e512e", "0x0e992462384c381ce6856381cfb7eb4d25bd94d4", "0x061ac6380baf6b065f0cf113e7f87aa85be5ab4212cd657925dc19a2bde7c93d"},
		{"0xb59cebfe149a60871ed8a8bc9a10e42b0edb853f", "0xb8d529c20d20dc87d9fab99bb9756d1036bf3ffd", "0x2dec22b919b430d9624f24626f7f20ce25a740db8ca429a9e6fe34ccd31fe6ca"},
		{"0xa56316f82def75dd95b630ddc762191483e5d16d", "0x87dda77d46b0603e7a5ac6d93bbbba97fa0aa5b1", "0xe89c14b001560aafad4162340027d95a657aa43ba05d7ff9ac0e91e08cef587b"},
		{"0x3d7623445d176d42037b78f12759ad6c9e5ae800", "0x007a4a562e2ba8f27dfc793d54d75902b13e20b2", "0x05ee6056514dd0181c2961c59f33ae7bc846521b905ae48fadbba8290e08707e"},
		{"0x69db451eb7e5e8c1de0926e05b2d421830762460", "0xff545fea675ae38a6e8381ac635b7dfa5b6f06dc", "0xd10ecd1677eb60c90146a687c53b92ff9a672f24587d02688b5431ff17551049"},
		{"0x2b012c300ef1454111e90dceae65d24fc1c2b829", "0xb6367ee36656db36b737a28122802c14c98fcf11", "0x2e5da2b1a8dbd3fe5e7e5850d4ee765137dbeb70908a76d54d0eb06c177ea092"},
		{"0xa4cc842dcf96a34f6d9f6bf45bd1b62c1cbbb906", "0x50f503bd872d90286cebcd9d35a36a9382926186", "0xcf3d2e45363923159a86d1fdf7c71eb83350e32582a396544e07203f9cfb97a9"},
		{"0xe14b57df5615029412a72b44dfeeb6d6be4580fa", "0x8b0cb6d1aa425fe3ff57d0d796c9cfa4d384024f", "0x4c1947ff385a75fde6b56ea5e27ba76e85f95c4a57975b5957867934a35d2ef7"},
		{"0x58a0150ffe9a84ed8ce3a8b2a91dca4d5963b02b", "0x59741509e614abbe3af30862df442e2196c563c9", "0x03097491e359aab712e651f59fc1a91577d206834c3f82a62e3ad25c5f7ef224"},
		{"0x9b0db6cc92e2bda08394219e6584b4c7e63f7f28", "0x7b683b0f5078a850b685b0c2f14262c86ea99b8f", "0xb9c00ee237b8a41d68380d60ef1cef14f9b29996ab584a3337a0247d96fc68d6"},
		{"0x1804f41dd5bb6448b1ebc72a64a5a3170e73d6eb", "0x57a4871d0b237f3cd0a108d84fc6ce6a5044b344", "0xe1eb41382137fb9a1596ae457beed0751ecde0370b00b622f6bd28de7290ae2c"},
		{"0x6df93dbc3acf146ed61a59486a20fb2e5d66d12e", "0xc3bfbad2b1a307a1ca7a0fa609f026051fbd3b49", "0x61d1b65787aafdd5033e01eb421847d70388372da272f7c92d52cd136fa94de7"},
		{"0xe0568c6410184bf5c6f2a5052d7ae30189b4afc7", "0xed7e1d9861aef4afc0e1697c7f2a38f18a98f162", "0x5eddd65b24b4ac213a52c628aabb9bf77ce99d45081bfb0596114854d2632daa"},
		{"0xef68e9899c8eee96957f3704d9ec67a33175f76a", "0x799c8b777777adc323ed59ebdf06795e8b07dd8d", "0x6670f5f2a32ca76627129d8b304a5ec7283a17d88c9fd6784ded13446b35d77c"},
		{"0x9e1c86df9e17451c0177544aa9a55edb3e0581b0", "0x0ffb1cbc86b3ad0a17292b7fda577d59c0d7aed9", "0x61dd053821c61c3f82dabf381f2abcca505d20b24b5d2a2b191fda977e4d934c"},
		{"0xcf689f5bd582e0978e93cd9a6729337ab5ad58e0", "0x3f29f03d3be2ac8ed855bc98485953901e8173ff", "0x89721d5c88242ea83870f9b0f86170b3693860064e8ee93c7dc2588909017202"},
		{"0x2d83c87339a87be5ffb5313f28c120a14accc546", "0xee25500c0f305cc42bdcb0f95e1d3186874bc19a", "0x6cb31685425106d45cf8a238af3f0563471cfeb38fe6f1fdff3d3715171cc2ed"},
		{"0x454055c76bc7381d1d8880a80e817691a9de1fd0", "0x7100a4dd6c3cf9c54aed649832a6c0170cf71329", "0x0a0840770af26c3521b9e27d10d261944c38cc5c74ddedada5379f6f9d2e2405"},
		{"0x97a78e92e7e6031bebb3a72bbe5e51eafe49f085", "0x00a8c9f77ef75ff0f6605a859bb32e16249f8341", "0xcd2a002fac3f676880483ec9c0374b5dd099d06a308d9deef10d812ec4a27ba3"},
		{"0x48bd7f2d9e82cfa36322add3b98a79df52553199", "0x4832b79985fb59095adc5534958c52762a54f462", "0xa6f7d9e393111aea6f777a54c7f5901a2217fa40cceeae6e52bd6a3cbf9fe096"},
		{"0xe214a04fc49f07e7c28a7e3145eba2ec36617204", "0xa7e43b445cf68caa143a884af673121447f29eae", "0x36e4c8ac2031b2c2018905f214949844e025b23a9c906e56834d5fde447fea6d"},
		{"0xc705ad4a9cb4130f05f0618586ede814aad73f29", "0xcbc6f8f1a6907841666fd4d5491194144ac75231", "0xaea1aa931793f96ff02669db805adb50d2c27b7120c5ff5e1d4f96b012fc1748"},
		{"0x3b4f4b8f58e572f79e12b03038f35d496c253667", "0x7a976d7160dadb5ed592b94ee0c79a17c47e82a2", "0x8566a1a5623c963126b95aacd8deb0c0800f33b181cb96b8d43b1113be94ea2e"},
		{"0x7b4165f9fdb56b22153097efeb50cfd4fa1840ed", "0x8e9c926b95b23fac411e152681f5cf8c6695a0b5", "0x11a1a86a20a38cb35b030353d907cb53d63c7d4a46f1f68281da81d26de53d4c"},
		{"0x4e86f6491485c379fcd10ee69a8808f787eac773", "0xa2966ac36bf0790cf6e280f07075045375fdec8c", "0x1662bc46076c9fd9c5ef65b4767df8c19f9c446b8f711387d0ffa544f73c719f"},
		{"0x14b384be90b2070d0402389820a28a32458e86d2", "0xdaab3408b7c80f73d1deaeafab8bf0eac2a4c217", "0xba8cbadec97d2cb24be754b2292fce092b39472bd2b1482cd12e46eb38c6b9df"},
		{"0xd1658f477b0ca15b190f0742a60f48b3996b31bf", "0x7276e812e560ebed03139e48b288867fccb47f10", "0xc547d9b55750258a814ca01a1d1caff67f6b639cc6bed1a1f43c03e5c86244b6"},
		{"0xbac07f33d09d5856182f430483c3df94077d0789", "0x28518e904c0bbed782b39fb7dd3dc24b99a65295", "0xf5bf71170433c4364f094c5484b41c47d4dd06880287921002544d9ab94256ba"},
	}

	candidates := make([]*authorityNode, 0, len(all))
	for _, item := range all {
		candidates = append(candidates, &authorityNode{
			masterAddress:   thor.MustParseAddress(item[0]),
			endorsorAddress: thor.MustParseAddress(item[1]),
			identity:        thor.MustParseBytes32(item[2]),
		})
	}
	return candidates
}
