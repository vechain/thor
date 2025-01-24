# Local Development - Built In Contracts

## Thor Solo

```bash
cd cmd/thor
go run . solo --api-cors="*" 
```

## Insight & Inspector

```bash
docker compose up --build
```

**Note**: If you restart thor, you may have to restart the explorer as well.

- Inspector: [http://localhost:8080](http://localhost:8080)
- Insights: [http://localhost:8081](http://localhost:8081)


## VeWorld

You can add the local network in VeWorld to submit transactions

- Open VeWorld -> Settings -> Networks
- Under `Other Networks` click `Add Network` and enter `http://localhost:8669`
- Note, if the genesis ID changes, you will have to repeat these steps
- **Optional**: Add the thor solo mnemonic to your wallet:

```
denial kitchen pet squirrel other broom bar gas better priority spoil cross
```
