import * as fs from 'fs'

const thorURLs = [
    'http://thor:8669',
    'http://host.docker.internal:8669',
    'http://localhost:8669',
]

const getGenesis = async () => {
    // retry all the thorURLs to get the genesisID
    for (const url of thorURLs) {
        try {
            const genesis = await fetch(`${url}/blocks/0`)
            return await genesis.json()
        } catch (e) {
            console.error(`Failed to get genesisID from: ${url}`)
        }
    }
    throw new Error("Failed to get genesisID from any thor URL")
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

    fs.mkdirSync("/usr/app/builtin/gen/abis", { recursive: true })
    fs.writeFileSync("/usr/app/builtin/gen/abis/contracts.json", JSON.stringify(abiConfig))
    fs.writeFileSync('/usr/app/genesis.json', JSON.stringify(genesis))

    console.log("ABI config file generated successfully")
}


main().catch((err) => {
    console.error(err)
    process.exit(1)
})
