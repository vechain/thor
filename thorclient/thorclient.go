// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package thorclient provides a comprehensive client for interacting with the VeChainThor blockchain.
//
// The client offers a complete set of methods to interact with:
//   - Accounts: Retrieve account information, balances, contract bytecode, and storage
//   - Transactions: Send, retrieve, and inspect transactions and their receipts
//   - Blocks: Access block information in both collapsed and expanded formats
//   - Events and Transfers: Filter and query smart contract events and VET transfers
//   - Network: Access peer information and the transaction pool if enabled by the node
//   - Fees: Retrieve historical fee data and priority fee estimations
//   - Subscriptions: Real-time updates via WebSocket for blocks, events, transfers, and more
//
// The client supports both HTTP and WebSocket connections, enabling both request/response
// patterns and real-time blockchain monitoring.
//
// Example usage:
//
//	client := thorclient.New("http://localhost:8669")
//	account, err := client.Account(addr)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Balance: %s\n", account.Balance)

package thorclient

import (
	"fmt"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/httpclient"
	"github.com/vechain/thor/v2/thorclient/wsclient"
	"github.com/vechain/thor/v2/tx"
)

// Client represents the VeChainThor blockchain client, providing comprehensive access
// to the VeChainThor REST API and WebSocket subscriptions.
//
// The client maintains both HTTP and WebSocket connections (when applicable) and provides
// methods that correspond to all available VeChainThor API endpoints. Methods are designed
// to be thread-safe and can be called concurrently.
//
// HTTP operations include account queries, transaction operations, block retrieval,
// event filtering, and administrative functions. WebSocket operations enable real-time
// subscriptions to blockchain events.
type Client struct {
	httpConn *httpclient.Client
	wsConn   *wsclient.Client
}

// New creates a new VeChainThor client using the provided HTTP URL.
//
// This constructor creates a client with HTTP-only capabilities. For WebSocket
// functionality, use NewWithWS instead.
//
// The URL should point to a VeChainThor node's REST API endpoint.
// Common examples:
//   - Mainnet: "https://mainnet.vechain.org"
//   - Testnet: "https://testnet.vechain.org"
//   - Local node: "http://localhost:8669"
//
// Parameters:
//   - url: The base URL of the VeChainThor node API
//
// Returns a new Client instance ready for HTTP operations.
func New(url string) *Client {
	return &Client{
		httpConn: httpclient.New(url),
	}
}

// NewWithHTTP creates a new VeChainThor client using the provided HTTP URL and custom HTTP client.
//
// This constructor allows you to provide a custom HTTP client with specific
// configurations such as timeouts, transport settings, or authentication.
//
// Parameters:
//   - url: The base URL of the VeChainThor node API
//   - c: A custom http.Client with desired configuration
//
// Returns a new Client instance using the custom HTTP client.
func NewWithHTTP(url string, c *http.Client) *Client {
	return &Client{
		httpConn: httpclient.NewWithHTTP(url, c),
	}
}

// NewWithWS creates a new VeChainThor client with both HTTP and WebSocket capabilities.
//
// This constructor enables full client functionality including real-time subscriptions
// to blockchain events. The same URL is used for both HTTP and WebSocket connections,
// with the WebSocket protocol automatically determined.
//
// WebSocket capabilities include subscriptions to:
//   - New blocks
//   - Smart contract events
//   - VET transfers
//   - Blockchain beats (block summaries with bloom filters)
//   - Transaction pool updates
//
// Parameters:
//   - url: The base URL of the VeChainThor node (used for both HTTP and WebSocket)
//
// Returns:
//   - *Client: A new client instance with both HTTP and WebSocket capabilities
//   - error: An error if the WebSocket connection cannot be established
func NewWithWS(url string) (*Client, error) {
	wsClient, err := wsclient.NewClient(url)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpConn: httpclient.New(url),
		wsConn:   wsClient,
	}, nil
}

// Option represents a functional option for customizing client requests.
//
// Options allow you to specify additional parameters for API calls such as
// block revision or whether to include pending transactions.
type Option func(*getOptions)

// getOptions holds configuration options for client requests.
type getOptions struct {
	revision string
	pending  bool
}

// applyOptions applies the given functional options to the default options.
func applyOptions(opts []Option) *getOptions {
	options := &getOptions{
		revision: httpclient.BestRevision,
		pending:  false,
	}
	for _, o := range opts {
		o(options)
	}
	return options
}

// applyHeadOptions applies the given functional options to the default options.
func applyHeadOptions(opts []Option) *getOptions {
	options := &getOptions{
		revision: "",
		pending:  false,
	}
	for _, o := range opts {
		o(options)
	}
	return options
}

// Revision returns an Option to specify the block revision for requests.
//
// The revision parameter can be:
//   - "best" - Latest block (default)
//   - "justified" - Latest justified block
//   - "finalized" - Latest finalized block
//   - A block number as string (e.g., "1000000")
//   - A block ID as hex string (e.g., "0x00...")
//
// For call operations, "next" is also supported on account inspections to execute on the next block.
//
// Parameters:
//   - revision: The block revision identifier
//
// Returns an Option that sets the revision for the request.
func Revision(revision string) Option {
	return func(o *getOptions) {
		o.revision = revision
	}
}

// Pending returns an Option to include pending transactions in the response.
//
// When this option is used with transaction queries, the response may include
// pending transactions that haven't been included in a block yet. Such transactions
// will have a null meta field.
//
// This option is useful for:
//   - Checking if a recently sent transaction is in the mempool
//   - Monitoring transaction status before block inclusion
//   - Real-time transaction tracking
//
// Returns an Option that enables pending transaction inclusion.
func Pending() Option {
	return func(o *getOptions) {
		o.pending = true
	}
}

// RawHTTPClient returns the underlying HTTP client for advanced use cases.
//
// This method provides direct access to the internal HTTP client, allowing
// for custom request handling or configuration that's not available through
// the high-level client methods.
//
// Returns the internal httpclient.Client instance.
func (c *Client) RawHTTPClient() *httpclient.Client {
	return c.httpConn
}

// RawWSClient returns the underlying WebSocket client for advanced use cases.
//
// This method provides direct access to the internal WebSocket client, allowing
// for custom subscription handling or configuration that's not available through
// the high-level client methods.
//
// Returns the internal wsclient.Client instance, or nil if the client was not
// created with WebSocket support (i.e., using New() instead of NewWithWS()).
func (c *Client) RawWSClient() *wsclient.Client {
	return c.wsConn
}

// Account retrieves account information from the VeChainThor blockchain.
//
// This method corresponds to the GET /accounts/{address} API endpoint and returns
// information about an account or contract, including:
//   - VET balance in wei (as hexadecimal string)
//   - Energy (VTHO) balance in wei (as hexadecimal string)
//   - Whether the account is a contract (hasCode boolean)
//
// The account information reflects the state at the specified block revision.
// To access historical account states, use the Revision() option.
//
// Parameters:
//   - addr: The VeChain address of the account/contract to query
//   - opts: Optional parameters (Revision)
//
// Returns:
//   - *api.Account: Account information including balance, energy, and contract status
//   - error: Error if the request fails or the address is invalid
//
// Example:
//
//	// Get current account state
//	account, err := client.Account(addr)
//
//	// Get account state at specific block
//	account, err := client.Account(addr, thorclient.Revision("1000000"))
func (c *Client) Account(addr *thor.Address, opts ...Option) (*api.Account, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccount(addr, options.revision)
}

// InspectClauses executes and inspects multiple contract calls or transactions without modifying blockchain state.
//
// This method corresponds to the POST /accounts/* API endpoint and can be used for:
//   - Reading contract state by calling view/pure functions
//   - Simulating transaction execution to check for potential reverts
//   - Inspecting transaction outputs before actual execution
//   - Estimating gas consumption (provide caller field for higher accuracy)
//   - Testing multi-clause transactions
//
// The method executes all clauses in the batch and returns detailed results including:
//   - Output data from contract calls
//   - Events generated during execution
//   - VET transfers that occurred
//   - Gas consumption for each clause
//   - Whether execution was reverted
//   - VM error messages if any
//
// For gas estimation, it's recommended to set revision to "next" and provide
// the caller field in the batch call data.
//
// Parameters:
//   - calldata: Batch of clauses to execute with optional gas, caller, and pricing info
//   - opts: Optional parameters (Revision - use "next" for gas estimation)
//
// Returns:
//   - []*api.CallResult: Results for each clause including data, events, transfers, and gas usage
//   - error: Error if the request fails or call data is invalid
//
// Example:
//
//	// Simulate a contract call
//	callData := &api.BatchCallData{
//		Clauses: api.Clauses{{
//			To:    &contractAddr,
//			Data:  "0x...", // encoded function call
//			Value: (*math.HexOrDecimal256)(big.NewInt(0)),
//		}},
//		Caller: &callerAddr,
//	}
//	results, err := client.InspectClauses(callData, thorclient.Revision("next"))
func (c *Client) InspectClauses(calldata *api.BatchCallData, opts ...Option) ([]*api.CallResult, error) {
	options := applyOptions(opts)
	return c.httpConn.InspectClauses(calldata, options.revision)
}

// InspectTxClauses inspects all clauses within a transaction without executing it on-chain.
//
// This is a convenience method that converts a VeChainThor transaction into the appropriate
// format for clause inspection. It accepts both signed and unsigned transactions and
// automatically extracts the necessary information for simulation.
//
// This method is particularly useful for:
//   - Pre-flight transaction validation
//   - Estimating gas costs for complex transactions
//   - Debugging transaction failures before sending
//   - Analyzing multi-clause transaction behavior
//
// The method internally converts the transaction to BatchCallData format and calls
// InspectClauses, providing the same detailed results.
//
// Parameters:
//   - tx: The VeChainThor transaction to inspect (signed or unsigned)
//   - senderAddr: The address of the transaction sender (used if transaction is unsigned)
//   - opts: Optional parameters (Revision - use "next" for gas estimation)
//
// Returns:
//   - []*api.CallResult: Results for each clause in the transaction
//   - error: Error if inspection fails or transaction is malformed
//
// Example:
//
//	// Inspect a transaction before sending
//	results, err := client.InspectTxClauses(transaction, senderAddr)
//	for i, result := range results {
//		if result.Reverted {
//			fmt.Printf("Clause %d would revert: %s\n", i, result.VmError)
//			return errors.New("transaction would revert")
//		}
//		fmt.Printf("Clause %d gas usage: %d\n", i, result.GasUsed)
//	}
func (c *Client) InspectTxClauses(tx *tx.Transaction, senderAddr *thor.Address, opts ...Option) ([]*api.CallResult, error) {
	clauses := convertToBatchCallData(tx, senderAddr)
	return c.InspectClauses(clauses, opts...)
}

// AccountCode retrieves the bytecode of a smart contract deployed at the specified address.
//
// This method corresponds to the GET /accounts/{address}/code API endpoint and returns
// the contract's bytecode in hexadecimal format. If the provided address is not a
// contract (i.e., it's a regular account), empty bytecode is returned.
//
// The bytecode represents the compiled smart contract code that is executed by the
// VeChainThor Virtual Machine. This is useful for:
//   - Verifying contract deployment
//   - Contract analysis and reverse engineering
//   - Checking if an address contains contract code
//   - Historical bytecode analysis at different block revisions
//
// Parameters:
//   - addr: The VeChain address of the contract to query
//   - opts: Optional parameters (Revision)
//
// Returns:
//   - *api.GetCodeResult: Contains the contract bytecode as a hex string
//   - error: Error if the request fails or address is invalid
//
// Example:
//
//	// Get contract bytecode
//	codeResult, err := client.AccountCode(contractAddr)
//	if err != nil {
//		return err
//	}
//	if codeResult.Code == "0x" {
//		fmt.Println("Address is not a contract")
//	} else {
//		fmt.Printf("Contract bytecode: %s\n", codeResult.Code)
//	}
func (c *Client) AccountCode(addr *thor.Address, opts ...Option) (*api.GetCodeResult, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccountCode(addr, options.revision)
}

// AccountStorage retrieves the value stored at a specific storage position in a smart contract.
//
// This method corresponds to the GET /accounts/{address}/storage/{key} API endpoint and
// allows you to read the raw storage of a smart contract at a specific storage slot.
// Each storage slot can hold a 32-byte value.
//
// Smart contract storage is organized as a key-value mapping where:
//   - Keys are 32-byte storage positions (usually determined by variable layout)
//   - Values are 32-byte data stored at those positions
//   - Storage layout depends on the contract's variable declarations and Solidity compiler version
//
// This is useful for:
//   - Reading contract state variables directly
//   - Analyzing contract storage layout
//   - Debugging smart contract behavior
//   - Historical storage analysis at different block revisions
//   - Bypassing contract getter functions
//
// Parameters:
//   - addr: The VeChain address of the contract
//   - key: The 32-byte storage position to read
//   - opts: Optional parameters (Revision)
//
// Returns:
//   - *api.GetStorageResult: Contains the 32-byte storage value as a hex string
//   - error: Error if the request fails or parameters are invalid
//
// Example:
//
//	// Read storage slot 0 (often used for the first state variable)
//	storageKey := thor.BytesToBytes32([]byte{0})
//	storageResult, err := client.AccountStorage(contractAddr, &storageKey)
//	if err != nil {
//		return err
//	}
//	fmt.Printf("Storage value: %s\n", storageResult.Value)
func (c *Client) AccountStorage(addr *thor.Address, key *thor.Bytes32, opts ...Option) (*api.GetStorageResult, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccountStorage(addr, key, options.revision)
}

func (c *Client) RawAccountStorage(addr *thor.Address, key *thor.Bytes32, opts ...Option) (*api.GetRawStorageResponse, error) {
	options := applyOptions(opts)
	return c.httpConn.GetRawAccountStorage(addr, key, options.revision)
}

// Transaction retrieves a transaction by its ID from the VeChainThor blockchain.
//
// This method corresponds to the GET /transactions/{id} API endpoint and returns
// comprehensive transaction information including all transaction fields and metadata.
// The transaction can be retrieved from both finalized blocks and the mempool (if pending).
//
// The returned transaction includes:
//   - Transaction fields (ID, origin, clauses, gas settings, etc.)
//   - Block metadata (block ID, number, timestamp) if confirmed
//   - Transaction type (0 for Legacy, 81 for DynamicFee)
//   - Fee information and delegation details
//
// Use the Pending() option to include transactions that are still in the mempool.
// Pending transactions will have a null meta field since they haven't been included
// in a block yet.
//
// Parameters:
//   - id: The 32-byte transaction ID to retrieve
//   - opts: Optional parameters (Pending)
//
// Returns:
//   - *transactions.Transaction: Complete transaction information with metadata
//   - error: Error if the request fails, transaction ID is invalid, or transaction not found
//
// Example:
//
//	// Get a confirmed transaction
//	tx, err := client.Transaction(txID)
//
//	// Check for pending transactions
//	tx, err := client.Transaction(txID, thorclient.Pending())
//	if tx != nil && tx.Meta == nil {
//		fmt.Println("Transaction is pending")
//	}
func (c *Client) Transaction(id *thor.Bytes32, opts ...Option) (*transactions.Transaction, error) {
	options := applyHeadOptions(opts)
	return c.httpConn.GetTransaction(id, options.revision, options.pending)
}

// RawTransaction retrieves a transaction in its raw RLP-encoded format by ID.
//
// This method corresponds to the GET /transactions/{id}?raw=true API endpoint and
// returns the transaction as it appears on the blockchain in RLP (Recursive Length
// Prefix) encoded format. This is useful for:
//   - Low-level transaction analysis
//   - Re-broadcasting transactions
//   - Cross-chain transaction verification
//   - Integration with external tools that require raw transaction data
//
// The raw format contains the exact bytes that were signed and submitted to the network,
// making it suitable for cryptographic verification and detailed protocol analysis.
//
// Parameters:
//   - id: The 32-byte transaction ID to retrieve
//   - opts: Optional parameters (Pending)
//
// Returns:
//   - *api.RawTransaction: Transaction in RLP-encoded hexadecimal format
//   - error: Error if the request fails, transaction ID is invalid, or transaction not found
//
// Example:
//
//	// Get raw transaction data
//	rawTx, err := client.RawTransaction(txID)
//	if err != nil {
//		return err
//	}
//	fmt.Printf("Raw transaction: %s\n", rawTx.Raw)
func (c *Client) RawTransaction(id *thor.Bytes32, opts ...Option) (*api.RawTransaction, error) {
	options := applyHeadOptions(opts)
	return c.httpConn.GetRawTransaction(id, options.revision, options.pending)
}

// TransactionReceipt retrieves the execution receipt for a transaction by its ID.
//
// This method corresponds to the GET /transactions/{id}/receipt API endpoint and
// returns detailed information about a transaction's execution results. Transaction
// receipts are only available for transactions that have been included in a block.
//
// The receipt contains:
//   - Execution status (reverted or successful)
//   - Gas consumption and payment details
//   - Transaction outputs for each clause
//   - Events generated during execution
//   - VET transfers that occurred
//   - Contract addresses for deployment transactions
//   - Block and transaction metadata
//
// This information is essential for:
//   - Confirming transaction success or failure
//   - Retrieving smart contract events
//   - Analyzing gas costs
//   - Getting deployed contract addresses
//   - Debugging failed transactions
//
// Parameters:
//   - id: The 32-byte transaction ID
//   - opts: Optional parameters (revision via head parameter)
//
// Returns:
//   - *api.Receipt: Complete transaction receipt with execution results
//   - error: Error if the request fails, transaction ID is invalid, or httpclient.ErrNotFound if receipt not found
//
// Example:
//
//	// Get transaction receipt
//	receipt, err := client.TransactionReceipt(txID)
//	if err != nil {
//		if errors.Is(err, httpclient.ErrNotFound) {
//			fmt.Println("Transaction receipt not found")
//			return nil
//		}
//		return err
//	}
//	if receipt.Reverted {
//		fmt.Println("Transaction failed")
//	} else {
//		fmt.Printf("Transaction succeeded, gas used: %d\n", receipt.GasUsed)
//	}
func (c *Client) TransactionReceipt(id *thor.Bytes32, opts ...Option) (*api.Receipt, error) {
	options := applyHeadOptions(opts)
	return c.httpConn.GetTransactionReceipt(id, options.revision)
}

// SendTransaction submits a signed transaction to the VeChainThor blockchain.
//
// This method corresponds to the POST /transactions API endpoint and broadcasts
// a signed transaction to the network for inclusion in a block. The transaction
// must be properly signed before submission.
//
// The method automatically handles RLP encoding of the transaction before submission.
// Once submitted successfully, the transaction enters the mempool and will be
// included in a future block by validators.
//
// Requirements for successful submission:
//   - Transaction must be properly signed
//   - Sender must have sufficient VET balance for value transfer
//   - Sender must have sufficient VTHO for gas fees (or delegated gas payment)
//   - Gas limit must be sufficient for execution
//   - Transaction must not be expired
//   - Nonce must be valid
//
// Parameters:
//   - tx: The signed VeChainThor transaction to submit
//
// Returns:
//   - *api.SendTxResult: Contains the transaction ID upon successful submission
//   - error: Error if submission fails (insufficient balance, invalid signature, etc.)
//
// Common errors:
//   - "insufficient balance for transfer" - Not enough VET
//   - "insufficient energy" - Not enough VTHO for gas
//   - "invalid signature" - Transaction signature is invalid
//   - "transaction expired" - Transaction has exceeded its expiration
//
// Example:
//
//	// Send a signed transaction
//	result, err := client.SendTransaction(signedTx)
//	if err != nil {
//		return fmt.Errorf("failed to send transaction: %v", err)
//	}
//	fmt.Printf("Transaction sent with ID: %s\n", result.ID)
func (c *Client) SendTransaction(tx *tx.Transaction) (*api.SendTxResult, error) {
	rlpTx, err := tx.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to encode transaction - %w", err)
	}

	return c.SendRawTransaction(rlpTx)
}

// SendRawTransaction submits a raw RLP-encoded transaction to the VeChainThor blockchain.
//
// This method corresponds to the POST /transactions API endpoint and accepts
// a transaction that has already been RLP-encoded. This is useful when:
//   - Working with transactions encoded by external tools
//   - Re-broadcasting previously serialized transactions
//   - Integrating with hardware wallets or external signers
//   - Handling transactions in their wire format
//
// The raw transaction must be a valid RLP-encoded, signed VeChainThor transaction.
// All the same requirements apply as with SendTransaction regarding balance,
// gas, signatures, and expiration.
//
// Parameters:
//   - rlpTx: The RLP-encoded transaction bytes
//
// Returns:
//   - *api.SendTxResult: Contains the transaction ID upon successful submission
//   - error: Error if submission fails or RLP data is invalid
//
// Example:
//
//	// Send raw transaction bytes
//	rawTxBytes := []byte{0xf8, 0x6c, ...} // RLP-encoded transaction
//	result, err := client.SendRawTransaction(rawTxBytes)
//	if err != nil {
//		return fmt.Errorf("failed to send raw transaction: %v", err)
//	}
//	fmt.Printf("Transaction sent with ID: %s\n", result.ID)
func (c *Client) SendRawTransaction(rlpTx []byte) (*api.SendTxResult, error) {
	return c.httpConn.SendTransaction(&api.RawTx{Raw: hexutil.Encode(rlpTx)})
}

// Block retrieves block information by its revision in collapsed format.
//
// This method corresponds to the GET /blocks/{revision} API endpoint and returns
// block information with transaction IDs only (not full transaction details).
// This is the most efficient way to retrieve block information when you don't
// need the complete transaction data.
//
// The block information includes:
//   - Block header fields (number, ID, timestamp, gas limit, etc.)
//   - Block metadata (size, parent ID, state roots)
//   - Signer information and consensus data
//   - List of transaction IDs (not full transactions)
//   - Block status (trunk/branch, finalized)
//
// For complete transaction details, use ExpandedBlock() instead.
//
// Parameters:
//   - revision: Block identifier - "best", "justified", "finalized", block number, or block ID
//
// Returns:
//   - *api.JSONCollapsedBlock: Block information with transaction ID list
//   - error: Error if the request fails, revision is invalid, or httpclient.ErrNotFound if block not found
//
// Example:
//
//	// Get the latest block
//	block, err := client.Block("best")
//	if err != nil {
//		if errors.Is(err, httpclient.ErrNotFound) {
//			fmt.Println("Block not found")
//			return nil
//		}
//		return err
//	}
//
//	// Get a specific block by number
//	block, err := client.Block("1000000")
//
//	// Get a specific block by ID
//	block, err := client.Block("0x00...")
func (c *Client) Block(revision string) (blocks *api.JSONCollapsedBlock, err error) {
	return c.httpConn.GetBlock(revision)
}

// ExpandedBlock retrieves block information by its revision with full transaction details.
//
// This method corresponds to the GET /blocks/{revision}?expanded=true API endpoint
// and returns complete block information including full transaction objects with
// their execution receipts. This provides the most comprehensive view of a block.
//
// The expanded block includes:
//   - All standard block header and metadata
//   - Complete transaction objects (not just IDs)
//   - Transaction receipts with execution results
//   - Events and transfers for each transaction
//   - Gas usage and fee payment details
//   - Contract deployment addresses
//
// This is more resource-intensive than Block() but provides complete information
// in a single request. Use this when you need full transaction details or when
// processing entire blocks.
//
// Parameters:
//   - revision: Block identifier - "best", "justified", "finalized", block number, or block ID
//
// Returns:
//   - *api.JSONExpandedBlock: Complete block information with full transaction details
//   - error: Error if the request fails, revision is invalid, or httpclient.ErrNotFound if block not found
//
// Example:
//
//	// Get expanded block with all transaction details
//	block, err := client.ExpandedBlock("best")
//	if err != nil {
//		return err
//	}
//	for i, tx := range block.Transactions {
//		fmt.Printf("Transaction %d: %s, Gas Used: %d\n", i, tx.ID, tx.GasUsed)
//	}
func (c *Client) ExpandedBlock(revision string) (blocks *api.JSONExpandedBlock, err error) {
	return c.httpConn.GetExpandedBlock(revision)
}

// FilterEvents queries smart contract events based on the provided filter criteria.
//
// This method corresponds to the POST /logs/event API endpoint and allows you to
// search for events generated by smart contracts using the LOG opcode in the EVM.
// Events provide a way to track specific occurrences and state changes within
// smart contracts.
//
// Filter capabilities include:
//   - Contract address filtering
//   - Event signature filtering (topic0)
//   - Indexed parameter filtering (topic1-4)
//   - Block range specification (by number or timestamp)
//   - Result pagination and ordering
//   - Historical event analysis
//
// The method supports complex filtering scenarios:
//   - Multiple criteria sets (OR logic between sets, AND within sets)
//   - Time-based or block-based ranges
//   - Ascending or descending result ordering
//   - Pagination for large result sets
//
// Results are limited to 1000 entries per query for performance.
//
// Parameters:
//   - req: Event filter request containing criteria, range, and options
//
// Returns:
//   - []api.FilteredEvent: Array of matching events with metadata
//   - error: Error if the request fails or filter criteria are invalid
//
// Example:
//
//	// Filter Transfer events from VTHO contract
//	filter := &api.EventFilter{
//		CriteriaSet: []api.EventCriteria{{
//			Address: &vthoContractAddr,
//			Topic0:  &transferEventSignature,
//		}},
//		Range: &api.FilterRange{
//			From: 1000000,
//			To:   1001000,
//		},
//	}
//	events, err := client.FilterEvents(filter)
func (c *Client) FilterEvents(req *api.EventFilter) ([]api.FilteredEvent, error) {
	return c.httpConn.FilterEvents(req)
}

// FilterTransfers queries VET transfer events based on the provided filter criteria.
//
// This method corresponds to the POST /logs/transfer API endpoint and allows you
// to search for VET transfers that occurred on the blockchain. Unlike smart contract
// events, VET transfers are native blockchain operations tracked separately.
//
// Filter capabilities include:
//   - Sender address filtering
//   - Recipient address filtering
//   - Transaction origin filtering
//   - Block range specification (by number or timestamp)
//   - Result pagination and ordering
//   - Historical transfer analysis
//
// VET transfers can occur in multiple scenarios:
//   - Direct VET transfers between accounts
//   - Contract-initiated transfers
//   - Multi-clause transaction transfers
//   - Gas fee payments (in some cases)
//
// The txOrigin field distinguishes the original transaction sender from the
// actual transfer sender (which may be a contract).
//
// Results are limited to 1000 entries per query for performance.
//
// Parameters:
//   - req: Transfer filter request containing criteria, range, and options
//
// Returns:
//   - []*api.FilteredTransfer: Array of matching VET transfers with metadata
//   - error: Error if the request fails or filter criteria are invalid
//
// Example:
//
//	// Filter transfers to a specific address
//	filter := &api.TransferFilter{
//		CriteriaSet: []api.TransferCriteria{{
//			Recipient: &targetAddr,
//		}},
//		Range: &api.FilterRange{
//			From: 1000000,
//			To:   1001000,
//		},
//	}
//	transfers, err := client.FilterTransfers(filter)
func (c *Client) FilterTransfers(req *api.TransferFilter) ([]*api.FilteredTransfer, error) {
	return c.httpConn.FilterTransfers(req)
}

// Peers retrieves information about all peers connected to the VeChainThor node.
//
// This method corresponds to the GET /node/network/peers API endpoint and returns
// information about each peer connection. This information is useful for:
//   - Network monitoring and diagnostics
//   - Understanding node connectivity
//   - Analyzing network topology
//   - Debugging synchronization issues
//
// For each connected peer, the response includes:
//   - Peer identification and software version
//   - Network address (IP:Port)
//   - Connection type (inbound/outbound)
//   - Best known block ID and total score
//   - Connection duration
//   - Unique peer ID on the network
//
// This method provides insight into the node's position in the VeChainThor
// peer-to-peer network and can help assess network health and connectivity.
//
// Returns:
//   - []*api.PeerStats: Array of peer connection statistics
//   - error: Error if the request fails or node information is unavailable
//
// Example:
//
//	peers, err := client.Peers()
//	if err != nil {
//		return err
//	}
//	fmt.Printf("Connected to %d peers:\n", len(peers))
//	for _, peer := range peers {
//		fmt.Printf("Peer: %s at %s (score: %d)\n", peer.Name, peer.NetAddr, peer.TotalScore)
//	}
func (c *Client) Peers() ([]*api.PeerStats, error) {
	return c.httpConn.GetPeers()
}

// ChainTag retrieves the chain tag identifier required for VeChainThor transaction construction.
//
// The chain tag is the last byte of the genesis block ID and serves as a unique
// identifier for the blockchain network. It must be included in every VeChainThor
// transaction to prevent cross-network replay attacks.
//
// Primary use case - Transaction construction:
//
//	When building VeChainThor transactions, the chain tag must be set in the
//	transaction body to ensure the transaction can only be executed on the
//	intended network (mainnet, testnet, or private network).
//
// Chain tag values:
//   - Mainnet: 0x4a
//   - Testnet: 0x27
//   - Solo/Custom: varies based on genesis configuration
//
// This method internally retrieves the genesis block (block 0) and extracts
// the chain tag from its block ID. The returned value should be used when
// constructing transactions.
//
// Returns:
//   - byte: The chain tag value (last byte of genesis block ID)
//   - error: Error if genesis block retrieval fails
//
// Example:
//
//	// Get chain tag for transaction construction
//	chainTag, err := client.ChainTag()
//	if err != nil {
//		return err
//	}
//
//	// Use chain tag when building transaction
//	tx := tx.NewBuilder(chainTag).
//		ChainTag(chainTag).
//		BlockRef(blockRef).
//		// ... other transaction fields
//		Build()
//
//	// Network identification
//	switch chainTag {
//	case 0x4a:
//		fmt.Println("Building transaction for mainnet")
//	case 0x27:
//		fmt.Println("Building transaction for testnet")
//	default:
//		fmt.Printf("Building transaction for network with chain tag: 0x%02x\n", chainTag)
//	}
func (c *Client) ChainTag() (byte, error) {
	genesisBlock, err := c.Block("0")
	if err != nil {
		return 0, err
	}
	return genesisBlock.ID[31], nil
}

// FeesHistory retrieves historical fee data for analysis and fee estimation.
//
// This method corresponds to the GET /fees/history API endpoint and returns
// fee-related information for a range of recent blocks. This data is essential
// for understanding network fee trends and making informed fee decisions.
//
// The method returns:
//   - Base fees per block (0x0 for blocks before Galactic fork)
//   - Gas used ratios (how full each block was)
//   - Reward percentiles (if requested) for priority fees
//   - Block range information
//
// This information helps with:
//   - Dynamic fee calculation
//   - Network congestion analysis
//   - Historical fee trend analysis
//   - Optimal fee estimation for transactions
//
// Note: Only accessible blocks are included (limited by api-backtrace-limit,
// default 1000 blocks from best block).
//
// Parameters:
//   - blockCount: Number of recent blocks to analyze (limited by node configuration)
//   - newestBlock: Starting block - "best", "justified", "finalized", or specific block
//   - rewardPercentiles: Optional percentiles for reward analysis (e.g., []float64{25, 50, 75})
//
// Returns:
//   - *api.FeesHistory: Historical fee data including base fees and gas usage ratios
//   - error: Error if the request fails or parameters are invalid
//
// Example:
//
//	// Get fee history for last 10 blocks with percentiles
//	percentiles := []float64{25.0, 50.0, 75.0}
//	history, err := client.FeesHistory(10, "best", percentiles)
//	if err != nil {
//		return err
//	}
//	fmt.Printf("Average gas used ratio: %.2f\n",
//			history.GasUsedRatio[len(history.GasUsedRatio)-1])
func (c *Client) FeesHistory(blockCount uint32, newestBlock string, rewardPercentiles []float64) (feesHistory *api.FeesHistory, err error) {
	return c.httpConn.GetFeesHistory(blockCount, newestBlock, rewardPercentiles)
}

// FeesPriority retrieves the suggested maximum priority fee for transaction inclusion.
//
// This method corresponds to the GET /fees/priority API endpoint and returns
// the recommended maxPriorityFeePerGas value to ensure a transaction is likely
// to be included in upcoming blocks by validators.
//
// The priority fee (tip) is paid to validators as an incentive to include the
// transaction in their proposed blocks. Higher priority fees increase the
// likelihood of faster transaction inclusion, especially during network congestion.
//
// This suggestion is based on:
//   - Recent network activity and fee patterns
//   - Current mempool conditions
//   - Validator behavior analysis
//   - Network congestion levels
//
// For DynamicFee transactions (type 81), use this value as maxPriorityFeePerGas.
// The total fee per gas will be baseFeePerGas + maxPriorityFeePerGas.
//
// Returns:
//   - *api.FeesPriority: Contains the suggested maxPriorityFeePerGas as hex string
//   - error: Error if the request fails or fee estimation is unavailable
//
// Example:
//
//	// Get priority fee suggestion
//	priority, err := client.FeesPriority()
//	if err != nil {
//		return err
//	}
//	fmt.Printf("Suggested priority fee: %s wei\n", priority.MaxPriorityFeePerGas)
//
//	// Use in transaction
//	// tx.MaxPriorityFeePerGas = priority.MaxPriorityFeePerGas
func (c *Client) FeesPriority() (feesPriority *api.FeesPriority, err error) {
	return c.httpConn.GetFeesPriority()
}

// SubscribeBlocks establishes a WebSocket subscription for real-time block updates.
//
// This method corresponds to the GET /subscriptions/block WebSocket endpoint and
// enables real-time monitoring of new blocks as they are added to the blockchain.
// Each message contains complete block information including transaction IDs.
//
// The subscription provides:
//   - Real-time block notifications
//   - Complete block header information
//   - List of transaction IDs in each block
//   - Block metadata (trunk/branch status, finalization)
//   - Obsolescence notifications for chain reorganizations
//
// This is essential for:
//   - Real-time blockchain monitoring
//   - DApp state synchronization
//   - Block explorer applications
//   - Transaction confirmation tracking
//   - Network analysis and monitoring
//
// The subscription can be resumed from a specific block position, allowing
// for reliable event processing even after disconnections.
//
// Parameters:
//   - pos: Block ID to resume from, or empty string for latest block
//
// Returns:
//   - *wsclient.Subscription[*api.BlockMessage]: Active block subscription with EventChan and Unsubscribe
//   - error: Error if WebSocket client unavailable or connection fails
//
// Note: Returns error if client was created without WebSocket support (using New()).
// Position too far behind may result in 403 error based on api-backtrace-limit.
//
// Example:
//
//	// Subscribe to new blocks
//	sub, err := client.SubscribeBlocks("")
//	if err != nil {
//		return err
//	}
//	defer func() {
//		if err := sub.Unsubscribe(); err != nil {
//			log.Printf("Error unsubscribing: %v", err)
//		}
//	}()
//
//	for wrapper := range sub.EventChan {
//		if wrapper.Error != nil {
//			return fmt.Errorf("subscription error: %v", wrapper.Error)
//		}
//		block := wrapper.Data
//		fmt.Printf("New block: %d with %d transactions\n",
//			block.Number, len(block.Transactions))
//	}
func (c *Client) SubscribeBlocks(pos string) (*wsclient.Subscription[*api.BlockMessage], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeBlocks(pos)
}

// SubscribeEvents establishes a WebSocket subscription for real-time smart contract event updates.
//
// This method corresponds to the GET /subscriptions/event WebSocket endpoint and
// enables real-time monitoring of smart contract events as they are emitted.
// Events are generated by smart contracts using the LOG opcode in the EVM.
//
// The subscription provides:
//   - Real-time event notifications
//   - Filtered events based on criteria
//   - Complete event data (address, topics, data)
//   - Block and transaction metadata
//   - Obsolescence notifications for chain reorganizations
//
// Filtering capabilities include:
//   - Contract address filtering
//   - Event signature filtering (topic0)
//   - Indexed parameter filtering (topic1-4)
//   - Multiple criteria support
//
// This is essential for:
//   - DApp real-time updates
//   - Smart contract monitoring
//   - Event-driven application logic
//   - Token transfer tracking
//   - DEX trade monitoring
//
// Parameters:
//   - pos: Block ID to resume from, or empty string for latest block
//   - filter: Event filtering criteria (address, topics)
//
// Returns:
//   - *wsclient.Subscription[*api.EventMessage]: Active event subscription with EventChan and Unsubscribe
//   - error: Error if WebSocket client unavailable or connection fails
//
// Example:
//
//	// Subscribe to Transfer events from VTHO contract
//	filter := &api.SubscriptionEventFilter{
//		Addr:   &vthoContractAddr,
//		Topic0: &transferEventSignature,
//	}
//	sub, err := client.SubscribeEvents("", filter)
//	if err != nil {
//		return err
//	}
//	defer func() {
//		if err := sub.Unsubscribe(); err != nil {
//			log.Printf("Error unsubscribing: %v", err)
//		}
//	}()
//
//	for wrapper := range sub.EventChan {
//		if wrapper.Error != nil {
//			return fmt.Errorf("subscription error: %v", wrapper.Error)
//		}
//		event := wrapper.Data
//		fmt.Printf("Transfer event: %s\n", event.Address)
//	}
func (c *Client) SubscribeEvents(pos string, filter *api.SubscriptionEventFilter) (*wsclient.Subscription[*api.EventMessage], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeEvents(pos, filter)
}

// SubscribeTransfers establishes a WebSocket subscription for real-time VET transfer updates.
//
// This method corresponds to the GET /subscriptions/transfer WebSocket endpoint and
// enables real-time monitoring of VET transfers as they occur on the blockchain.
// Unlike smart contract events, VET transfers are native blockchain operations.
//
// The subscription provides:
//   - Real-time VET transfer notifications
//   - Filtered transfers based on criteria
//   - Complete transfer data (sender, recipient, amount)
//   - Transaction origin information
//   - Block and transaction metadata
//   - Obsolescence notifications for chain reorganizations
//
// Filtering capabilities include:
//   - Sender address filtering
//   - Recipient address filtering
//   - Transaction origin filtering
//   - Multiple criteria support
//
// This is essential for:
//   - Wallet balance monitoring
//   - Payment processing applications
//   - Exchange deposit/withdrawal tracking
//   - VET flow analysis
//   - Real-time payment notifications
//
// Parameters:
//   - pos: Block ID to resume from, or empty string for latest block
//   - filter: Transfer filtering criteria (sender, recipient, txOrigin)
//
// Returns:
//   - *wsclient.Subscription[*api.TransferMessage]: Active transfer subscription with EventChan and Unsubscribe
//   - error: Error if WebSocket client unavailable or connection fails
//
// Example:
//
//	// Subscribe to transfers to a specific address
//	filter := &api.SubscriptionTransferFilter{
//		Recipient: &walletAddr,
//	}
//	sub, err := client.SubscribeTransfers("", filter)
//	if err != nil {
//		return err
//	}
//	defer func() {
//		if err := sub.Unsubscribe(); err != nil {
//			log.Printf("Error unsubscribing: %v", err)
//		}
//	}()
//
//	for wrapper := range sub.EventChan {
//		if wrapper.Error != nil {
//			return fmt.Errorf("subscription error: %v", wrapper.Error)
//		}
//		transfer := wrapper.Data
//		fmt.Printf("Incoming VET: %s from %s\n", transfer.Amount, transfer.Sender)
//	}
func (c *Client) SubscribeTransfers(pos string, filter *api.SubscriptionTransferFilter) (*wsclient.Subscription[*api.TransferMessage], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeTransfers(pos, filter)
}

// SubscribeBeats2 establishes a WebSocket subscription for blockchain beat notifications with enhanced data.
//
// This method corresponds to the GET /subscriptions/beat2 WebSocket endpoint and
// provides real-time blockchain beats containing block summaries with bloom filters
// and enhanced block information including gas limit and base fee data.
//
// Beat2 messages include:
//   - Basic block information (number, ID, timestamp)
//   - Parent block ID and transaction features
//   - Bloom filter for efficient address/topic checking
//   - Gas limit and base fee per gas (enhanced over beat1)
//   - Bloom filter parameters (k value)
//   - Obsolescence notifications
//
// The bloom filter enables efficient checking of whether specific addresses
// or topics are affected by the block without downloading full block data.
// This is particularly useful for light clients and selective data processing.
//
// This subscription is ideal for:
//   - Light client implementations
//   - Efficient block monitoring
//   - Address-specific change detection
//   - Network statistics collection
//   - Gas price tracking
//
// Parameters:
//   - pos: Block ID to resume from, or empty string for latest block
//
// Returns:
//   - *wsclient.Subscription[*api.Beat2Message]: Active beat2 subscription with EventChan and Unsubscribe
//   - error: Error if WebSocket client unavailable or connection fails
//
// Example:
//
//	// Subscribe to blockchain beats
//	sub, err := client.SubscribeBeats2("")
//	if err != nil {
//		return err
//	}
//	defer func() {
//		if err := sub.Unsubscribe(); err != nil {
//			log.Printf("Error unsubscribing: %v", err)
//		}
//	}()
//
//	for wrapper := range sub.EventChan {
//		if wrapper.Error != nil {
//			return fmt.Errorf("subscription error: %v", wrapper.Error)
//		}
//		beat := wrapper.Data
//		fmt.Printf("New block %d, gas limit: %d, base fee: %s\n",
//			beat.Number, beat.GasLimit, beat.BaseFeePerGas)
//	}
func (c *Client) SubscribeBeats2(pos string) (*wsclient.Subscription[*api.Beat2Message], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeBeats2(pos)
}

// SubscribeTxPool establishes a WebSocket subscription for transaction pool updates.
//
// This method corresponds to the GET /subscriptions/txpool WebSocket endpoint and
// enables real-time monitoring of transactions entering the mempool (pending
// transactions awaiting block inclusion).
//
// The subscription provides:
//   - Real-time notifications of new pending transactions
//   - Transaction IDs as they enter the pool
//   - Mempool activity monitoring
//   - Network activity insights
//
// This is useful for:
//   - Mempool analysis and monitoring
//   - Transaction broadcast verification
//   - Network congestion analysis
//   - MEV (Maximum Extractable Value) opportunities
//   - Real-time transaction tracking
//
// Note: The current implementation accepts a txID parameter but this may be
// used for filtering specific transactions or reserved for future functionality.
//
// Parameters:
//   - txID: Transaction ID filter (may be nil for all transactions)
//
// Returns:
//   - *wsclient.Subscription[*api.PendingTxIDMessage]: Active txpool subscription with EventChan and Unsubscribe
//   - error: Error if WebSocket client unavailable or connection fails
//
// Example:
//
//	// Subscribe to all pending transactions
//	sub, err := client.SubscribeTxPool(nil)
//	if err != nil {
//		return err
//	}
//	defer func() {
//		if err := sub.Unsubscribe(); err != nil {
//			log.Printf("Error unsubscribing: %v", err)
//		}
//	}()
//
//	for wrapper := range sub.EventChan {
//		if wrapper.Error != nil {
//			return fmt.Errorf("subscription error: %v", wrapper.Error)
//		}
//		msg := wrapper.Data
//		fmt.Printf("New pending transaction: %s\n", msg.ID)
//	}
func (c *Client) SubscribeTxPool(txID *thor.Bytes32) (*wsclient.Subscription[*api.PendingTxIDMessage], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeTxPool(txID)
}

func convertToBatchCallData(tx *tx.Transaction, addr *thor.Address) *api.BatchCallData {
	cls := make(api.Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = convertClauseAccounts(c)
	}

	blockRef := tx.BlockRef()
	encodedBlockRef := hexutil.Encode(blockRef[:])

	return &api.BatchCallData{
		Clauses:    cls,
		Gas:        tx.Gas(),
		ProvedWork: nil, // todo hook this field
		Caller:     addr,
		GasPayer:   nil, // todo hook this field
		GasPrice:   nil, // todo hook this field
		Expiration: tx.Expiration(),
		BlockRef:   encodedBlockRef,
	}
}

func convertClauseAccounts(c *tx.Clause) *api.Clause {
	value := math.HexOrDecimal256(*c.Value())
	return &api.Clause{
		To:    c.To(),
		Value: &value,
		Data:  hexutil.Encode(c.Data()),
	}
}
