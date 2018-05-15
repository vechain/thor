package main

import "github.com/ethereum/go-ethereum/p2p/discover"

var bootstrapNodes = []*discover.Node{
	discover.MustParseNode("enode://a8a83b4faac13f0a05ecd383d661a85e15e2a93fb41c4b5d00976d0bb8e35aab58a6303fe6b437124888da45017b94df8ce72f6a8bb5bcfdc7bd8df51698ad01@106.75.226.133:55555"),
	discover.MustParseNode("enode://e42edaa9bee0c324ffd63600d435dc22b88f777aaacadcdb257110e81d57fb4e796b9277ff81f367d578ff9521525a89b78e21332ba7081a7322899a9f352837@106.75.226.228:55555"),
	discover.MustParseNode("enode://194ca15507248b26d28be32041fb27bc671d28dfc703f046906edbf64a03ea2bffe65ee79e15ded68208eeec1fbbd406ae5f1090a5f98c9f58cd1f16abdb80be@107.150.109.14:55555"),
	discover.MustParseNode("enode://b02d26a3a18abcc57829d5fbf27fec8c38c25763a5793a17b85fad0c5ac573c9e24ed1a937d9f4bd1a580e2b3dc837fb13cd6ea06116c791f522f61a708f305b@128.14.224.37:55555"),
}
