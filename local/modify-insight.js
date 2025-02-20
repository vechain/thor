const fs = require('fs')

const APP_JS = "/usr/app/html/js/app.8762cf7a.js"

const OLD_GENESIS = '{number:0,id:"0x00000000c05a20fbca2bf6ae3affba6af4a74b800b585bf7a4988aba7aea69f6",size:170,parentID:"0xffffffff53616c757465202620526573706563742c20457468657265756d2100",timestamp:1530316800,gasLimit:1e7,beneficiary:"0x0000000000000000000000000000000000000000",gasUsed:0,totalScore:0,txsRoot:"0x45b0cfc220ceec5b7c1c62c4d4193d38e4eba48e8815729ce75f9c0ab0e4c1c0",txsFeatures:0,stateRoot:"0x93de0ffb1f33bc0af053abc2a87c4af44594f5dcb1cb879dd823686a15d68550",receiptsRoot:"0x45b0cfc220ceec5b7c1c62c4d4193d38e4eba48e8815729ce75f9c0ab0e4c1c0",signer:"0x0000000000000000000000000000000000000000",isTrunk:!0,transactions:[]}'


const thorURL = process.env.VUE_APP_SOLO_URL

const getGenesis = async () => {
    try {
        console.log(`Trying to get genesisID from: ${thorURL}`)
        const genesis = await fetch(`${thorURL}/blocks/0`)
        console.log(`Got genesisID from: ${thorURL}`)
        return await genesis.json()
    } catch (e) {
        console.error(`Failed to get genesisID from: ${thorURL}`)
        throw new Error("Failed to get genesisID from any thor URL")
    }
}

const main = async () => {
    const genesis = await getGenesis()

    const app = fs.readFileSync(APP_JS, 'utf8')
    const newApp = app.replace(OLD_GENESIS, JSON.stringify(genesis))


    fs.writeFileSync(APP_JS, newApp)
}

main().catch((err) => {
    console.error(err)
    process.exit(1)
})
