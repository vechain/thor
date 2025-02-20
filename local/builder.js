import * as fs from 'fs'

const thorURL = process.env.VUE_APP_SOLO_URL

const getGenesis = async () => {
    // retry all the thorURLs to get the genesisID
    try {
        console.log(`Trying to get genesisID from: ${thorURL}`)
        const genesis = await fetch(`${thorURL}/blocks/0`)
        console.log(`Got genesisID from: ${thorURL}`)
        return await genesis.json()
    } catch (e) {
        console.error(`Failed to get genesisID from: ${thorURL}`)
        throw new Error("Failed to get genesisID from all thorURLs")
    }
}

const main = async () => {

    console.log("Generating ABI config file..")

    const files = fs.readdirSync('/usr/app/builtin/gen/compiled')
        .filter(file => file.endsWith('.abi') && !file.toLowerCase().includes("native"))

    const names = files.map(file => file.split('.')[0])

    const addresses = names.map(name => {
        const bytes = Buffer.from(name, 'utf8')
        return '0x' + bytes.toString('hex').padStart(40, '0')
    })

    const abis = files.map((file) => {
        const fileData = fs.readFileSync(`/usr/app/builtin/gen/compiled/${file}`)
        return JSON.parse(fileData.toString())
    })

    const genesis = await getGenesis()

    console.log(`Genesis ID: ${genesis.id}`)

    const abiConfig = names.map((name, index) => {
        return {
            genesisId: genesis.id,
            name,
            address: addresses[index],
            abi: abis[index]
        }
    })

    fs.mkdirSync("/usr/app/html/abis", { recursive: true })
    fs.writeFileSync("/usr/app/html/abis/contracts.json", JSON.stringify(abiConfig))
    fs.writeFileSync('/usr/app/genesis.json', JSON.stringify(genesis))

    console.log("ABI config file generated successfully")
}


main().catch((err) => {
    console.error(err)
    process.exit(1)
})
