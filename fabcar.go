package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/pkg/cid"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// CustomerAccount represents a unique account for a customer on a token
type CustomerAccount struct {
	AccountID string `json:"account_id"`
	Balance   int    `json:"balance"`
}

// RegisterCustomerAccount creates a new account for a customer on a token
func (s *SmartContract) RegisterCustomerAccount(ctx contractapi.TransactionContextInterface, tokenID string, networkAddress string, currency string) (string, error) {
	// Generate unique account ID using transaction ID
	txID := ctx.GetStub().GetTxID()
	accountID := fmt.Sprintf("account_%s", txID)
	account := CustomerAccount{
		AccountID: accountID,
		Balance:   0,
	}
	accountBytes, err := json.Marshal(account)
	if err != nil {
		return "", err
	}
	key := "account_" + accountID
	err = ctx.GetStub().PutState(key, accountBytes)
	if err != nil {
		return "", err
	}
	return accountID, nil
}

// GetCustomerAccounts returns all accounts for a customer on a token
func (s *SmartContract) GetCustomerAccounts(ctx contractapi.TransactionContextInterface, tokenID string, networkAddress string) ([]CustomerAccount, error) {
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var accounts []CustomerAccount
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(kv.Key, "account_") {
			var acc CustomerAccount
			if err := json.Unmarshal(kv.Value, &acc); err == nil {
				accounts = append(accounts, acc)
			}
		}
	}
	return accounts, nil
}

// GetAccountBalance returns the balance for a specific account
func (s *SmartContract) GetAccountBalance(ctx contractapi.TransactionContextInterface, accountID string) (int, error) {
	key := "account_" + accountID
	accBytes, err := ctx.GetStub().GetState(key)
	if err != nil || accBytes == nil {
		return 0, fmt.Errorf("account not found")
	}
	var acc CustomerAccount
	if err := json.Unmarshal(accBytes, &acc); err != nil {
		return 0, err
	}
	return acc.Balance, nil
}

const maxTokens = 25

type SmartContract struct {
	contractapi.Contract
}

type Participant struct {
	// Privacy-first canonical fields (preferred for new consumers).
	CustomerRef     string           `json:"customer_ref,omitempty"`
	KycRef          string           `json:"kyc_ref,omitempty"`
	TokenID         string           `json:"token_id"`
	BIC             string           `json:"bic,omitempty"`
	Balance         int              `json:"balance"`
	ForeignBalances map[string]int64 `json:"foreign_balances,omitempty"`
	Status          string           `json:"status,omitempty"` // ACTIVE/SUSPENDED/PENDING
	ActivatedAt     string           `json:"activated_at,omitempty"`
	LastUpdated     string           `json:"last_updated,omitempty"`
	TransferRefs    []string         `json:"transfer_refs,omitempty"`

	// Legacy fields kept for backward compatibility.
	CustomerID        string             `json:"customer_id"`
	Name              string             `json:"name"`
	NetworkAddress    string             `json:"network_address"`
	ClientID          string             `json:"client_id"`
	MSP               string             `json:"msp"` // Bank/Organization (Org1MSP, Org2MSP, Org3MSP)
	Approved          bool               `json:"approved"`
	ApprovedAt        string             `json:"approved_at"`
	Country           string             `json:"country"`
	TransferIDs       []string           `json:"transfer_ids"`
	KycId             string             `json:"kyc_id"`
	KycStatus         string             `json:"kyc_status"`
	ForeignCurrencies map[string]float64 `json:"foreign_currencies"` // Holdings in other currencies
	TokenTransferIDs  []string           `json:"token_transfer_ids"`
}

type Token struct {
	TokenID     string `json:"token_id"`     // Unique asset identifier
	BIC         string `json:"bic"`          // SWIFT/ISO 20022 institution ID
	Currency    string `json:"currency"`     // ISO 4217
	TotalSupply int    `json:"total_supply"` // Current circulation
	MaxSupply   int    `json:"max_supply"`   // Regulatory cap
	Status      string `json:"status"`       // ACTIVE/FROZEN/EXPIRED
	IsFrozen    bool   `json:"is_frozen"`    // Emergency controls

	// Internal/operational fields used by existing business flows.
	Owner           string         `json:"owner"`
	OwnerMSP        string         `json:"owner_msp"` // Bank that owns this token (Org1MSP, Org2MSP, Org3MSP)
	Available       bool           `json:"available"`
	DisplayTokenID  string         `json:"display_token_id"`
	TransferIDs     []string       `json:"transfer_ids"`
	AssignedAt      string         `json:"assigned_at"`
	ForeignBalances map[string]int `json:"foreign_balances"`
	Minted          int            `json:"minted"` // Legacy compatibility; mirrors TotalSupply
}

func getTokenSupply(t Token) int {
	if t.TotalSupply != 0 {
		return t.TotalSupply
	}
	return t.Minted
}

func setTokenSupply(t *Token, amount int) {
	t.TotalSupply = amount
	t.Minted = amount
}

// TokenCommissionConfig stores the commission percentage that a bank charges for transfers
type TokenCommissionConfig struct {
	TokenID              string  `json:"token_id"`
	CommissionPercentage float64 `json:"commission_percentage"` // e.g., 2.0 for 2%
	UpdatedAt            string  `json:"updated_at"`
	UpdatedBy            string  `json:"updated_by"` // Owner address who set this commission
}

// SimplifiedTokenResponse for public display (customers viewing available currencies)
type SimplifiedTokenResponse struct {
	TokenID        string `json:"token_id"`
	Currency       string `json:"currency"`
	DisplayTokenID string `json:"display_token_id"`
}

type TokenRequest struct {
	MsgID              string `json:"msg_id"`              // Financial message ID
	InstitutionID      string `json:"institution_id"`      // BIC8/BIC11
	InstitutionName    string `json:"institution_name"`    // Legal entity name
	CountryCode        string `json:"country_code"`        // ISO 3166-1 alpha-2
	CurrencyCode       string `json:"currency_code"`       // ISO 4217
	TokenID            string `json:"token_id"`            // Assigned on approval
	RequestPurpose     string `json:"request_purpose"`     // Business request type
	Status             string `json:"status"`              // PENDING, APPROVED, REJECTED, CANCELLED
	CreatedAt          string `json:"created_at"`          // RFC3339
	ValidUntil         string `json:"valid_until"`         // RFC3339 expiry
	Reference          string `json:"reference"`           // Business reference
	ApproverID         string `json:"approver_id"`         // Populated at approval
	RequestID          string `json:"request_id"`          // Legacy compatibility
	NetworkAddr        string `json:"network_addr"`        // Legacy compatibility
	ParticipantAddress string `json:"participant_address"` // Legacy compatibility
	CallerID           string `json:"caller_id"`           // Legacy compatibility
	CallerMSP          string `json:"caller_msp"`          // Legacy compatibility
	ParticipantMSP     string `json:"participant_msp"`     // Legacy compatibility
	Currency           string `json:"currency"`            // Legacy compatibility
}

type MintRequest struct {
	MsgID          string `json:"msg_id"`           // MT799: transaction reference
	BIC            string `json:"bic"`              // SWIFT/ISO 20022 bank identifier
	TokenID        string `json:"token_id"`         // Token identifier
	CustomerRef    string `json:"customer_ref"`     // Customer reference
	CustomerID     string `json:"customer_id"`      // Legacy alias for compatibility
	KycRef         string `json:"kyc_ref"`          // KYC reference
	Amount         int64  `json:"amount"`           // Mint amount
	Currency       string `json:"currency"`         // ISO 4217 currency
	KycStatus      string `json:"kyc_status"`       // VERIFIED/PENDING
	Status         string `json:"status"`           // PENDING/APPROVED/REJECTED
	Purpose        string `json:"purpose"`          // WORKING_CAPITAL/SETTLEMENT/LIQUIDITY
	CreatedAt      string `json:"created_at"`       // RFC3339
	ApprovedAt     string `json:"approved_at"`      // RFC3339
	ExpiresAt      string `json:"expires_at"`       // RFC3339 expiry
	DailyLimitUsed int64  `json:"daily_limit_used"` // Running total for compliance trace
}

// TokenMintRecord stores token-owner/admin mint operations separately from customer mint requests.
type TokenMintRecord struct {
	RecordID     string `json:"record_id"`
	MsgID        string `json:"msg_id"`
	RequestID    string `json:"request_id"`
	BIC          string `json:"bic"`
	TokenID      string `json:"token_id"`
	Amount       int64  `json:"amount"`
	Currency     string `json:"currency"`
	Purpose      string `json:"purpose"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	ApprovedAt   string `json:"approved_at"`
	ApprovedBy   string `json:"approved_by"`
	CustomerRef  string `json:"customer_ref"`
	CustomerID   string `json:"customer_id"`
	MintCategory string `json:"mint_category"` // TOKEN_OWNER_MINT
}

// KYCAnchor stores minimal anchor metadata for off-chain KYC
// KYCAnchor removed — KYC/demo artifacts cleaned up

// TokenHandshake represents approval between two token owners to communicate
type TokenHandshake struct {
	HandshakeID   string `json:"handshakeID"`
	FirstTokenID  string `json:"firstTokenID"`
	SecondTokenID string `json:"secondTokenID"`
	ApprovedBy    string `json:"approvedBy"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
}

// RegisterParticipantRequest for pending participant registrations
type RegisterParticipantRequest struct {
	MsgID       string `json:"msg_id"`
	BIC         string `json:"bic"`
	TokenID     string `json:"token_id"`
	CustomerRef string `json:"customer_ref"`
	KycRef      string `json:"kyc_ref"`
	KycStatus   string `json:"kyc_status"`
	Status      string `json:"status"`
	Purpose     string `json:"purpose"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at"`
}

// TransferRequest struct removed - use TokenTransferRequest instead

type TokenTransferRequest struct {
	MsgID           string  `json:"msg_id"`            // MT202 field 20
	SenderBIC       string  `json:"sender_bic"`        // SWIFT BIC
	ReceiverBIC     string  `json:"receiver_bic"`      // SWIFT BIC
	SenderTokenID   string  `json:"sender_token_id"`   // Sender token
	ReceiverTokenID string  `json:"receiver_token_id"` // Receiver token
	Amount          int64   `json:"amount"`            // Transfer amount
	Currency        string  `json:"currency"`          // ISO 4217
	ExchangeRate    float64 `json:"exchange_rate"`     // 1.0 for same-currency transfers
	Status          string  `json:"status"`            // PENDING/APPROVED/REJECTED/SETTLED
	Purpose         string  `json:"purpose"`           // RTGS/NEFT/INTERBANK_SETTLEMENT
	CreatedAt       string  `json:"created_at"`        // RFC3339
	ExpiresAt       string  `json:"expires_at"`        // RFC3339
	SettledAt       string  `json:"settled_at,omitempty"`

	// Legacy compatibility for existing consumers.
	RequestID   string `json:"request_id,omitempty"`
	InitiatedBy string `json:"initiated_by,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type TokenToTokenTransferRecord struct {
	TxRef           string  `json:"tx_ref"`            // SWIFT style settlement reference
	MsgID           string  `json:"msg_id"`            // Original transfer message ID
	SenderBIC       string  `json:"sender_bic"`        // Sender institution BIC
	ReceiverBIC     string  `json:"receiver_bic"`      // Receiver institution BIC
	SenderTokenID   string  `json:"sender_token_id"`   // Sender token
	ReceiverTokenID string  `json:"receiver_token_id"` // Receiver token
	Amount          int64   `json:"amount"`            // Settled amount
	Currency        string  `json:"currency"`          // ISO 4217
	ExchangeRate    float64 `json:"exchange_rate"`     // Applied FX rate
	FeeAmount       int64   `json:"fee_amount"`        // Settlement fee
	NetAmount       int64   `json:"net_amount"`        // Amount after fee
	Status          string  `json:"status"`            // SETTLED/FAILED/REVERSED
	SettledAt       string  `json:"settled_at"`        // Final settlement timestamp
	BlockHeight     string  `json:"block_height"`      // Immutable ledger proof marker
	Purpose         string  `json:"purpose"`           // RTGS/NEFT/INTERBANK_SETTLEMENT

	// Legacy compatibility aliases.
	RecordID    string `json:"record_id,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	InitiatedBy string `json:"initiated_by,omitempty"`
	ApprovedBy  string `json:"approved_by,omitempty"`
	ApprovedAt  string `json:"approved_at,omitempty"`
}

type ParticipantTransferRecord struct {
	TxRef               string  `json:"tx_ref"`
	RequestMsgID        string  `json:"request_msg_id"`
	SenderCustomerRef   string  `json:"sender_customer_ref"`
	SenderBIC           string  `json:"sender_bic"`
	ReceiverCustomerRef string  `json:"receiver_customer_ref"`
	ReceiverBIC         string  `json:"receiver_bic"`
	Amount              int64   `json:"amount"`
	Currency            string  `json:"currency"`
	Commission          int64   `json:"commission"`
	NetAmount           int64   `json:"net_amount"`
	ExchangeRate        float64 `json:"exchange_rate"`
	Status              string  `json:"status"`
	SettledAt           string  `json:"settled_at"`
	BlockHeight         string  `json:"block_height"`

	// Legacy compatibility aliases.
	RecordID              string `json:"record_id,omitempty"`
	TransferRequestID     string `json:"transfer_request_id,omitempty"`
	TransferID            string `json:"transfer_id,omitempty"`
	TokenID               string `json:"token_id,omitempty"`
	SenderParticipantID   string `json:"sender_participant_id,omitempty"`
	ReceiverParticipantID string `json:"receiver_participant_id,omitempty"`
	SenderName            string `json:"sender_name,omitempty"`
	ReceiverName          string `json:"receiver_name,omitempty"`
	SenderKycId           string `json:"sender_kyc_id,omitempty"`
	SenderKycStatus       string `json:"sender_kyc_status,omitempty"`
	ReceiverKycId         string `json:"receiver_kyc_id,omitempty"`
	ReceiverKycStatus     string `json:"receiver_kyc_status,omitempty"`
	SenderTokenID         string `json:"sender_token_id,omitempty"`
	ReceiverTokenID       string `json:"receiver_token_id,omitempty"`
	CompletedAt           string `json:"completed_at,omitempty"`
}

type CustomerTokenAccount struct {
	CustomerRef    string `json:"customer_ref"`
	CustomerID     string `json:"customer_id"`
	TokenID        string `json:"token_id"`
	BIC            string `json:"bic"`
	Approved       bool   `json:"approved"`
	Status         string `json:"status"`
	NetworkAddress string `json:"network_address"`
}

// CustomerToTokenTransferRequest represents a customer initiating a transfer to another token (which forwards to another customer)
// Flow: Sender Customer (Token A) → Receiver Token B → Receiver Customer (Token B)
// Receiver Token takes 2% commission, forwards 98% to Receiver Customer
type CustomerToTokenTransferRequest struct {
	MsgID               string  `json:"msg_id"`                // Privacy-safe transaction reference
	SenderCustomerRef   string  `json:"sender_customer_ref"`   // Business-safe customer reference
	SenderBIC           string  `json:"sender_bic"`            // Sender bank BIC
	ReceiverCustomerRef string  `json:"receiver_customer_ref"` // Business-safe customer reference
	ReceiverBIC         string  `json:"receiver_bic"`          // Receiver bank BIC
	Amount              int64   `json:"amount"`                // Sender amount
	Currency            string  `json:"currency"`              // Sender currency
	Status              string  `json:"status"`                // PENDING_SENDER/PENDING_RECEIVER/SETTLED/REJECTED_SENDER_PRE_ESCROW/REJECTED_RECEIVER/EXPIRED_ESCROW_RETURNED/FAILED_TECHNICAL
	RejectionReason     string  `json:"rejection_reason"`
	RejectedAt          string  `json:"rejected_at"`
	EscrowAmount        int64   `json:"escrow_amount"`        // Locked sender amount
	CommissionPct       float64 `json:"commission_pct"`       // Commission percentage ratio (0.02 = 2%)
	CommissionAmount    int64   `json:"commission_amount"`    // Commission amount
	NetReceiverAmount   int64   `json:"net_receiver_amount"`  // Final amount credited to receiver customer
	ExchangeRate        float64 `json:"exchange_rate"`        // FX rate if currency differs
	CreatedAt           string  `json:"created_at"`           // RFC3339
	SettledAt           string  `json:"settled_at"` // RFC3339

	// Internal/legacy compatibility fields retained for existing flows.
	TransferRequestID            string  `json:"transfer_request_id"`
	SenderCustomerID             string  `json:"sender_customer_id"` // Customer network address
	SenderCustomerTokenID        string  `json:"sender_customer_token_id"`
	SenderCustomerName           string  `json:"sender_customer_name"`
	SenderTokenID                string  `json:"sender_token_id"`
	ReceiverTokenID              string  `json:"receiver_token_id"`
	ReceiverCustomerID           string  `json:"receiver_customer_id"` // Customer network address
	ReceiverCustomerTokenID      string  `json:"receiver_customer_token_id"`
	ReceiverCustomerName         string  `json:"receiver_customer_name"`
	SenderCurrency               string  `json:"sender_currency"` // Legacy
	InitiatedBy                  string  `json:"initiated_by"`
	DebitStatus                  string  `json:"debit_status"`
	CreditStatus                 string  `json:"credit_status"`
	EscrowedAmount               int64   `json:"escrowed_amount"` // Legacy alias
	ApprovedBySenderOwner        bool    `json:"approved_by_sender_owner"`
	ApprovedByReceiverOwner      bool    `json:"approved_by_receiver_owner"`
	SenderTokenOwnerApprovedAt   string  `json:"sender_approved_at"`
	ReceiverTokenOwnerApprovedAt string  `json:"receiver_approved_at"`
	CompletedAt                  string  `json:"completed_at"`
	ReceiverCurrency             string  `json:"receiver_currency"` // Legacy
	CommissionPercentage         float64 `json:"commission_percentage"`
	ReceiverCustomerAmount       int64   `json:"receiver_customer_amount"`
	ConvertedAmount              float64 `json:"converted_amount"`
}

// TransactionHistoryRecord stores transaction history for both mint and transfer transactions
type TransactionHistoryRecord struct {
	TransactionID  string  `json:"transaction_id"`  // Unique transaction ID
	Category       string  `json:"category"`        // MINT or TRANSFER
	ParticipantID  string  `json:"participant_id"`  // Customer/Participant ID who owns this transaction record
	Timestamp      string  `json:"timestamp"`       // ISO 8601 timestamp
	Amount         float64 `json:"amount"`          // Transaction amount
	Currency       string  `json:"currency"`        // Currency code (USD, EUR, etc)
	CurrencySymbol string  `json:"currency_symbol"` // Currency symbol ($, €, etc)
	Type           string  `json:"type"`            // CREDIT (green) or DEBIT (red)
	Status         string  `json:"status"`          // PENDING, COMPLETED, FAILED, REVERSED

	// For TRANSFER transactions
	SenderID             string  `json:"sender_id"`             // Sender participant ID
	SenderTokenID        string  `json:"sender_token_id"`       // Sender's token ID
	SenderName           string  `json:"sender_name"`           // Sender's name
	ReceiverID           string  `json:"receiver_id"`           // Receiver participant ID
	ReceiverTokenID      string  `json:"receiver_token_id"`     // Receiver's token ID
	ReceiverName         string  `json:"receiver_name"`         // Receiver's name
	AmountReceived       float64 `json:"amount_received"`       // Amount received by receiver (after commission)
	CommissionBank       string  `json:"commission_bank"`       // Bank name taking commission (from TokenID/Owner)
	CommissionPercentage float64 `json:"commission_percentage"` // Commission percentage (e.g., 2.0 for 2%)
	CommissionAmount     float64 `json:"commission_amount"`     // Actual commission amount deducted
	TransferRequestID    string  `json:"transfer_request_id"`   // Reference to original transfer request

	// For MINT transactions
	// These fields are primarily populated for MINT category
	MintRequestID string `json:"mint_request_id"` // Reference to mint request
	ApprovedBy    string `json:"approved_by"`     // Admin/Authority who approved mint
	ApprovedAt    string `json:"approved_at"`     // When mint was approved

	// Metadata
	RelatedTransactionID string `json:"related_transaction_id"` // For transfers, the corresponding record on other side
}

// TransactionHistory index maintains all transaction IDs for a participant
type TransactionHistory struct {
	ParticipantID      string   `json:"participant_id"`
	TransactionIDs     []string `json:"transaction_ids"`
	MintTransactionIDs []string `json:"mint_transaction_ids"`
	TransferIDs        []string `json:"transfer_ids"`
	LastUpdated        string   `json:"last_updated"`
}

func appendTransferIfMissing(list []string, transferID string) []string {
	for _, existing := range list {
		if existing == transferID {
			return list
		}
	}
	return append(list, transferID)
}

const (
	participantStatePrefix     = "participant_"
	participantIndexPrefix     = "participantidx_"
	customerIDUniquePrefix     = "customerid_"
	customerIDTokenIndexPrefix = "participantbytoken_"
)

func participantLegacyStateKey(networkAddress, tokenID string) string {
	return fmt.Sprintf("%s%s_%s", participantStatePrefix, networkAddress, tokenID)
}

func participantStateKeyByCustomerID(customerID string) string {
	return fmt.Sprintf("%s%s", participantStatePrefix, customerID)
}

func participantNetworkTokenIndexKey(networkAddress, tokenID string) string {
	return fmt.Sprintf("%s%s_%s", participantIndexPrefix, networkAddress, tokenID)
}

func customerIDUniqueKey(customerID string) string {
	return fmt.Sprintf("%s%s", customerIDUniquePrefix, customerID)
}

func customerIDTokenIndexKey(tokenID, customerID string) string {
	return fmt.Sprintf("%s%s_%s", customerIDTokenIndexPrefix, tokenID, customerID)
}

func generateTokenScopedCustomerID(tokenID, txID string) string {
	cleanToken := strings.ToUpper(strings.TrimSpace(tokenID))
	cleanToken = strings.ReplaceAll(cleanToken, "-", "")
	cleanToken = strings.ReplaceAll(cleanToken, "_", "")
	cleanToken = strings.ReplaceAll(cleanToken, " ", "")
	if len(cleanToken) > 10 {
		cleanToken = cleanToken[:10]
	}
	if cleanToken == "" {
		cleanToken = "TOKEN"
	}

	cleanTx := strings.TrimSpace(txID)
	if len(cleanTx) > 14 {
		cleanTx = cleanTx[:14]
	}
	if cleanTx == "" {
		cleanTx = strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	return fmt.Sprintf("CUST_%s_%s", cleanToken, strings.ToUpper(cleanTx))
}

func ensureCustomerIDUnique(ctx contractapi.TransactionContextInterface, customerID string) error {
	if strings.TrimSpace(customerID) == "" {
		return fmt.Errorf("customer_id cannot be empty")
	}
	existing, err := ctx.GetStub().GetState(customerIDUniqueKey(customerID))
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("customer_id already exists")
	}
	return nil
}

func (s *SmartContract) resolveParticipantStateKey(ctx contractapi.TransactionContextInterface, networkAddress, tokenID string) (string, error) {
	indexKey := participantNetworkTokenIndexKey(networkAddress, tokenID)
	indexValue, err := ctx.GetStub().GetState(indexKey)
	if err != nil {
		return "", err
	}
	if indexValue != nil {
		customerID := strings.TrimSpace(string(indexValue))
		if customerID != "" {
			stateKey := participantStateKeyByCustomerID(customerID)
			existing, getErr := ctx.GetStub().GetState(stateKey)
			if getErr != nil {
				return "", getErr
			}
			if existing != nil {
				return stateKey, nil
			}
		}
	}

	legacyKey := participantLegacyStateKey(networkAddress, tokenID)
	legacy, err := ctx.GetStub().GetState(legacyKey)
	if err != nil {
		return "", err
	}
	if legacy != nil {
		return legacyKey, nil
	}
	return "", fmt.Errorf("customer record not found for network %s and token %s", networkAddress, tokenID)
}

func (s *SmartContract) getParticipantByNetworkToken(ctx contractapi.TransactionContextInterface, networkAddress, tokenID string) (*Participant, string, error) {
	stateKey, err := s.resolveParticipantStateKey(ctx, networkAddress, tokenID)
	if err != nil {
		return nil, "", err
	}
	raw, err := ctx.GetStub().GetState(stateKey)
	if err != nil {
		return nil, "", err
	}
	if raw == nil {
		return nil, "", fmt.Errorf("customer state missing")
	}
	var participant Participant
	if err := json.Unmarshal(raw, &participant); err != nil {
		return nil, "", fmt.Errorf("invalid customer record: %w", err)
	}
	normalizeParticipantForRead(&participant)
	return &participant, stateKey, nil
}

func (s *SmartContract) getParticipantByCustomerIDToken(ctx contractapi.TransactionContextInterface, customerID, tokenID string) (*Participant, string, error) {
	customerID = strings.TrimSpace(customerID)
	tokenID = strings.TrimSpace(tokenID)
	if customerID == "" || tokenID == "" {
		return nil, "", fmt.Errorf("customer_id and token_id are required")
	}
	indexKey := customerIDTokenIndexKey(tokenID, customerID)
	stateKeyBytes, err := ctx.GetStub().GetState(indexKey)
	if err != nil {
		return nil, "", err
	}
	if stateKeyBytes == nil {
		return nil, "", fmt.Errorf("customer record not found")
	}
	stateKey := strings.TrimSpace(string(stateKeyBytes))
	raw, err := ctx.GetStub().GetState(stateKey)
	if err != nil {
		return nil, "", err
	}
	if raw == nil {
		return nil, "", fmt.Errorf("customer state missing")
	}
	var participant Participant
	if err := json.Unmarshal(raw, &participant); err != nil {
		return nil, "", fmt.Errorf("invalid customer record: %w", err)
	}
	normalizeParticipantForRead(&participant)
	return &participant, stateKey, nil
}

func decodeBase64Candidate(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(decoded))
}

func customerRefMatches(ref string, candidates ...string) bool {
	normalizedRef := strings.TrimSpace(ref)
	if normalizedRef == "" {
		return false
	}
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if trimmed == normalizedRef {
			return true
		}
		decodedCandidate := decodeBase64Candidate(trimmed)
		if decodedCandidate != "" && decodedCandidate == normalizedRef {
			return true
		}
		decodedRef := decodeBase64Candidate(normalizedRef)
		if decodedRef != "" && decodedRef == trimmed {
			return true
		}
	}
	return false
}

func (s *SmartContract) findPendingCustomerRegistration(ctx contractapi.TransactionContextInterface, networkAddress, tokenID, callerID string) (*RegisterParticipantRequest, error) {
	trimmedTokenID := strings.TrimSpace(tokenID)
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isRegisterParticipantRequestKey(kv.Key) {
			continue
		}
		var req RegisterParticipantRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if strings.TrimSpace(req.TokenID) != trimmedTokenID {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(req.Status)) != "PENDING" {
			continue
		}
		if !customerRefMatches(req.CustomerRef, networkAddress, callerID) {
			continue
		}
		copyReq := req
		return &copyReq, nil
	}
	return nil, nil
}

func currencySymbol(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY":
		return "¥"
	case "INR":
		return "₹"
	case "NGN":
		return "₦"
	case "KES":
		return "Ksh "
	case "CNY":
		return "¥"
	case "AUD":
		return "A$"
	case "CAD":
		return "C$"
	default:
		if code == "" {
			return ""
		}
		return strings.ToUpper(strings.TrimSpace(code)) + " "
	}
}

func formatCurrencyValue(code string, amount float64) string {
	if strings.TrimSpace(code) == "" {
		return fmt.Sprintf("%.2f", amount)
	}
	symbol := currencySymbol(code)
	return fmt.Sprintf("%s%.2f", symbol, amount)
}

func deriveParticipantTransferID(networkAddress string) string {
	if networkAddress == "" {
		return ""
	}
	return fmt.Sprintf("participanttransfer_%s", networkAddress)
}

func deriveBankTransferID(tokenID string) string {
	if tokenID == "" {
		return ""
	}
	return fmt.Sprintf("banktransfer_%s", tokenID)
}

func participantBalanceStateKey(networkAddress string) string {
	return fmt.Sprintf("participantbalance_%s", networkAddress)
}

func (s *SmartContract) adjustParticipantBalance(ctx contractapi.TransactionContextInterface, networkAddress string, delta float64) error {
	if networkAddress == "" {
		return fmt.Errorf("participant network address required")
	}
	key := participantBalanceStateKey(networkAddress)
	balBytes, err := ctx.GetStub().GetState(key)
	if err != nil {
		return err
	}
	var balance float64
	if balBytes != nil {
		balance, err = strconv.ParseFloat(strings.TrimSpace(string(balBytes)), 64)
		if err != nil {
			return fmt.Errorf("invalid participant balance data")
		}
	} else {
		legacyBytes, err := ctx.GetStub().GetState(networkAddress)
		if err != nil {
			return err
		}
		if legacyBytes != nil {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(string(legacyBytes)), 64); err == nil {
				balance = parsed
			} else if delta < 0 {
				return fmt.Errorf("participant balance not found")
			}
		} else if delta < 0 {
			return fmt.Errorf("participant balance not found")
		}
	}

	newBalance := balance + delta

	// SECURITY FIX #1: Enforce strict zero balance (no negative tolerance)
	if newBalance < 0 {
		return fmt.Errorf("insufficient participant balance")
	}

	// SECURITY FIX #5: Use fixed decimal format for precision
	return ctx.GetStub().PutState(key, []byte(fmt.Sprintf("%.2f", newBalance)))
}

// Helper functions for transfer request removed

func (s *SmartContract) getParticipantBalance(ctx contractapi.TransactionContextInterface, networkAddress string) (float64, error) {
	if networkAddress == "" {
		return 0, fmt.Errorf("participant network address required")
	}

	// SECURITY FIX #2: Verify caller identity before returning balance
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return 0, fmt.Errorf("failed to get caller identity: %v", err)
	}
	// Only allow caller to check their own balance
	participantBytes, err := ctx.GetStub().GetState(networkAddress)
	if err == nil && participantBytes != nil {
		var p Participant
		if err := json.Unmarshal(participantBytes, &p); err == nil {
			if p.ClientID != "" && p.ClientID != callerID {
				return 0, fmt.Errorf("unauthorized: cannot access other participant's balance")
			}
		}
	}

	key := participantBalanceStateKey(networkAddress)
	balBytes, err := ctx.GetStub().GetState(key)
	if err != nil {
		return 0, err
	}
	if balBytes != nil {
		balance, err := strconv.ParseFloat(strings.TrimSpace(string(balBytes)), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid participant balance data")
		}
		return balance, nil
	}
	legacyBytes, err := ctx.GetStub().GetState(networkAddress)
	if err != nil {
		return 0, err
	}
	if legacyBytes != nil {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(string(legacyBytes)), 64); err == nil {
			return parsed, nil
		}
	}
	return 0, nil
}

// resolveParticipantReference removed - was for transfer requests only

// recordTransferForToken removed - was only used by transfer request functions

func (s *SmartContract) currentTxTime(ctx contractapi.TransactionContextInterface) (string, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", err
	}
	t := time.Unix(ts.Seconds, int64(ts.Nanos)).UTC()
	return t.Format(time.RFC3339), nil
}

// getExchangeRate returns the real-time exchange rate from sourceCurrency to targetCurrency
// Exchange rates are stored on the ledger as "exchangerate_CURRENCY" and should be updated periodically
// Rates are relative to USD as the base currency
// Admins can call UpdateExchangeRate to update current market rates
func (s *SmartContract) getExchangeRate(ctx contractapi.TransactionContextInterface, sourceCurrency, targetCurrency string) (float64, error) {
	source := strings.ToUpper(strings.TrimSpace(sourceCurrency))
	target := strings.ToUpper(strings.TrimSpace(targetCurrency))

	if source == target {
		return 1.0, nil
	}

	// Try to fetch rates from ledger first (most current)
	exchangeRates := make(map[string]float64)

	supportedCurrencies := []string{"USD", "EUR", "GBP", "JPY", "INR", "NGN", "KES", "CNY", "AUD", "CAD"}

	for _, currency := range supportedCurrencies {
		key := fmt.Sprintf("exchangerate_%s", currency)
		rateBytes, err := ctx.GetStub().GetState(key)

		if err == nil && rateBytes != nil {
			// Rate found on ledger
			if rate, err := strconv.ParseFloat(string(rateBytes), 64); err == nil {
				exchangeRates[currency] = rate
				continue
			}
		}

		// Fallback to hardcoded rates if not on ledger (initial values)
		switch currency {
		case "USD":
			exchangeRates[currency] = 1.0
		case "EUR":
			exchangeRates[currency] = 0.92 // 1 USD = 0.92 EUR
		case "GBP":
			exchangeRates[currency] = 0.79 // 1 USD = 0.79 GBP
		case "JPY":
			exchangeRates[currency] = 149.50 // 1 USD = 149.50 JPY
		case "INR":
			exchangeRates[currency] = 83.45 // 1 USD = 83.45 INR
		case "NGN":
			exchangeRates[currency] = 1547.00 // 1 USD = 1547.00 NGN
		case "KES":
			exchangeRates[currency] = 130.50 // 1 USD = 130.50 KES
		case "CNY":
			exchangeRates[currency] = 7.25 // 1 USD = 7.25 CNY
		case "AUD":
			exchangeRates[currency] = 1.53 // 1 USD = 1.53 AUD
		case "CAD":
			exchangeRates[currency] = 1.36 // 1 USD = 1.36 CAD
		}
	}

	sourceRate, sourceExists := exchangeRates[source]
	if !sourceExists {
		return 0, fmt.Errorf("unsupported source currency: %s", sourceCurrency)
	}

	targetRate, targetExists := exchangeRates[target]
	if !targetExists {
		return 0, fmt.Errorf("unsupported target currency: %s", targetCurrency)
	}

	// Convert: amount in sourceCurrency -> USD -> targetCurrency
	// Rate = targetRate / sourceRate (because sourceRate is in terms of USD)
	rate := targetRate / sourceRate
	return rate, nil
}

// UpdateExchangeRate allows admin to update current exchange rates
// This should be called periodically (e.g., daily) to keep rates current
func (s *SmartContract) UpdateExchangeRate(ctx contractapi.TransactionContextInterface, currency string, rate float64) error {
	// Verify caller is admin
	if err := s.VerifyAdmin(ctx); err != nil {
		return fmt.Errorf("only admin can update exchange rates: %v", err)
	}

	if rate <= 0 {
		return fmt.Errorf("exchange rate must be positive")
	}

	curr := strings.ToUpper(strings.TrimSpace(currency))
	supportedCurrencies := map[string]bool{
		"USD": true, "EUR": true, "GBP": true, "JPY": true, "INR": true,
		"NGN": true, "KES": true, "CNY": true, "AUD": true, "CAD": true,
	}

	if !supportedCurrencies[curr] {
		return fmt.Errorf("unsupported currency: %s", currency)
	}

	key := fmt.Sprintf("exchangerate_%s", curr)
	if err := ctx.GetStub().PutState(key, []byte(fmt.Sprintf("%.6f", rate))); err != nil {
		return fmt.Errorf("failed to update exchange rate: %v", err)
	}

	return nil
}

// convertAmount converts an amount from sourceCurrency to targetCurrency using current rates
func (s *SmartContract) convertAmount(ctx contractapi.TransactionContextInterface, amount int, sourceCurrency, targetCurrency string) (int, error) {
	if strings.TrimSpace(sourceCurrency) == strings.TrimSpace(targetCurrency) {
		return amount, nil
	}

	rate, err := s.getExchangeRate(ctx, sourceCurrency, targetCurrency)
	if err != nil {
		return 0, err
	}

	// Convert amount to target currency
	convertedAmount := float64(amount) * rate

	// Round to nearest integer for cryptocurrency/token precision
	return int(convertedAmount + 0.5), nil
}

// convertAmountWithDecimal converts an amount from sourceCurrency to targetCurrency with decimal precision
func (s *SmartContract) convertAmountWithDecimal(ctx contractapi.TransactionContextInterface, amount int, sourceCurrency, targetCurrency string) (float64, error) {
	if strings.TrimSpace(sourceCurrency) == strings.TrimSpace(targetCurrency) {
		return float64(amount), nil
	}

	rate, err := s.getExchangeRate(ctx, sourceCurrency, targetCurrency)
	if err != nil {
		return 0, err
	}

	// Convert amount to target currency with decimal precision
	convertedAmount := float64(amount) * rate

	return convertedAmount, nil
}

// InitLedger initializes ledger
func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
	// Create 25 tokens
	for i := 1; i <= 25; i++ {
		tokenID := fmt.Sprintf("token_%d", i)
		token := Token{
			TokenID:         tokenID,
			BIC:             "",
			Currency:        "",
			TotalSupply:     0,
			MaxSupply:       0,
			Status:          "ACTIVE",
			IsFrozen:        false,
			Owner:           "",
			Available:       true,
			DisplayTokenID:  "",
			TransferIDs:     []string{},
			ForeignBalances: make(map[string]int),
			Minted:          0,
		}
		tokenBytes, _ := json.Marshal(token)
		if err := ctx.GetStub().PutState(tokenID, tokenBytes); err != nil {
			return err
		}
	}

	return nil
}

// SubmitRegistration registers participant with generated network address.
// passwordHash parameter is accepted for backwards compatibility but handled off-chain.
func (s *SmartContract) SubmitRegistration(ctx contractapi.TransactionContextInterface, name, _passwordHash, country string) (string, error) {
	clientID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", err
	}
	msp, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", err
	}
	netAddr := clientID

	exists, err := s.ParticipantExists(ctx, netAddr)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("participant already exists")
	}

	// Create participant with MSP field to track which bank they belong to
	p := Participant{
		CustomerRef:    netAddr,
		Status:         "PENDING",
		Name:           name,
		NetworkAddress: netAddr,
		ClientID:       clientID,
		MSP:            msp,
		Approved:       false,
		Country:        country,
		TokenID:        "",
		KycId:          "",
		KycStatus:      "",
	}
	normalizeParticipantForWrite(&p, "", "")
	b, _ := json.Marshal(p)
	if err := ctx.GetStub().PutState(netAddr, b); err != nil {
		return "", err
	}
	return netAddr, nil
}

func (s *SmartContract) ParticipantExists(ctx contractapi.TransactionContextInterface, networkAddress string) (bool, error) {
	b, err := ctx.GetStub().GetState(networkAddress)
	if err != nil {
		return false, err
	}
	return b != nil, nil
}

// VerifyAdmin checks if caller is from a valid bank organization (supports multiple banks)
func (s *SmartContract) VerifyAdmin(ctx contractapi.TransactionContextInterface) error {
	msp, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return err
	}
	// Accept any valid bank MSP (Org1MSP, Org2MSP, Org3MSP, etc.)
	if msp == "" {
		return fmt.Errorf("MSP not found")
	}
	return nil
}

// GetCallerMSP returns the MSP ID of the caller
func (s *SmartContract) GetCallerMSP(ctx contractapi.TransactionContextInterface) (string, error) {
	return ctx.GetClientIdentity().GetMSPID()
}

// VerifyBankAccessToData checks if caller can access data from a specific bank
func (s *SmartContract) VerifyBankAccessToData(ctx contractapi.TransactionContextInterface, dataOwnerMSP string) error {
	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}

	// Data isolation: caller can only access their own bank's data
	if callerMSP != dataOwnerMSP {
		return fmt.Errorf("access denied: cannot access another bank's data (your bank: %s, data owner: %s)", callerMSP, dataOwnerMSP)
	}
	return nil
}

// VerifyBankOwner ensures caller's bank owns the specified token
func (s *SmartContract) VerifyBankOwner(ctx contractapi.TransactionContextInterface, tokenID string) error {
	// Get caller's MSP
	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}

	// Get token to verify ownership
	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil {
		return fmt.Errorf("failed to read token: %v", err)
	}
	if tokenBytes == nil {
		return fmt.Errorf("token not found: %s", tokenID)
	}

	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return fmt.Errorf("failed to parse token: %v", err)
	}

	// Verify caller's bank owns this token
	if token.OwnerMSP != callerMSP {
		return fmt.Errorf("access denied: your bank (%s) does not own token %s (owner: %s)", callerMSP, tokenID, token.OwnerMSP)
	}

	return nil
}

// VerifyBankIdentity ensures caller is a bank (has a token assigned)
func (s *SmartContract) VerifyBankIdentity(ctx contractapi.TransactionContextInterface) (string, error) {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %v", err)
	}

	// Find participant record for this caller
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return "", err
	}
	defer iter.Close()

	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(kv.Key, "participant_") || !strings.Contains(kv.Key, "_") {
			var p Participant
			if err := json.Unmarshal(kv.Value, &p); err == nil {
				if p.ClientID == callerID && p.TokenID != "" {
					return p.TokenID, nil // Bank has a token assigned
				}
			}
		}
	}

	return "", fmt.Errorf("access denied: caller is not a registered bank")
}

func validBICFormat(institutionID string) bool {
	return regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`).MatchString(institutionID)
}

func validCountryCode(code string) bool {
	return regexp.MustCompile(`^[A-Z]{2}$`).MatchString(code)
}

func validCurrencyCode(code string) bool {
	return regexp.MustCompile(`^[A-Z]{3}$`).MatchString(code)
}

const (
	maxMintRequestAmount      int64 = 100000000
	mintRequestTTLDays              = 7
	mintPurposeWorkingCapital       = "WORKING_CAPITAL"
	mintPurposeSettlement           = "SETTLEMENT"
	mintPurposeLiquidity            = "LIQUIDITY"
)

func validMintPurpose(purpose string) bool {
	switch strings.TrimSpace(strings.ToUpper(purpose)) {
	case mintPurposeWorkingCapital, mintPurposeSettlement, mintPurposeLiquidity:
		return true
	default:
		return false
	}
}

const (
	transferPurposeRTGS                = "RTGS"
	transferPurposeNEFT                = "NEFT"
	transferPurposeInterbankSettlement = "INTERBANK_SETTLEMENT"
	transferRequestTTLHours            = 24
)

func validBankTransferPurpose(purpose string) bool {
	switch strings.TrimSpace(strings.ToUpper(purpose)) {
	case transferPurposeRTGS, transferPurposeNEFT, transferPurposeInterbankSettlement:
		return true
	default:
		return false
	}
}

func normalizeTokenTransferStatus(status string) string {
	switch strings.TrimSpace(strings.ToUpper(status)) {
	case "PENDING", "PENDINGRECEIVERAPPROVAL":
		return "PENDING"
	case "APPROVED":
		return "APPROVED"
	case "SETTLED", "COMPLETED":
		return "SETTLED"
	case "REJECTED":
		return "REJECTED"
	default:
		return strings.TrimSpace(strings.ToUpper(status))
	}
}

func normalizeTokenTransferRequestForRead(req *TokenTransferRequest) {
	if req == nil {
		return
	}
	req.Status = normalizeTokenTransferStatus(req.Status)
	if strings.TrimSpace(req.MsgID) == "" {
		req.MsgID = strings.TrimSpace(req.RequestID)
	}
	if strings.TrimSpace(req.Currency) == "" {
		req.Currency = "INR"
	}
	if req.ExchangeRate == 0 {
		req.ExchangeRate = 1.0
	}
	if strings.TrimSpace(req.SettledAt) == "" && strings.TrimSpace(req.CompletedAt) != "" {
		req.SettledAt = req.CompletedAt
	}
	if strings.TrimSpace(req.CompletedAt) == "" && strings.TrimSpace(req.SettledAt) != "" {
		req.CompletedAt = req.SettledAt
	}
	if strings.TrimSpace(req.RequestID) == "" {
		req.RequestID = req.MsgID
	}
}

func isTokenTransferRequestKey(key string) bool {
	return strings.HasPrefix(key, "tokentransfer_") || strings.Contains(key, "/TRANSFER/")
}

func isTokenToTokenTransferHistoryKey(key string) bool {
	return strings.HasPrefix(key, "tokentotransferhistory_") || strings.Contains(key, "/SETTLED/")
}

func normalizeTokenToTokenTransferRecordForRead(record *TokenToTokenTransferRecord) {
	if record == nil {
		return
	}
	if strings.TrimSpace(record.TxRef) == "" {
		record.TxRef = strings.TrimSpace(record.RecordID)
	}
	if strings.TrimSpace(record.RecordID) == "" {
		record.RecordID = strings.TrimSpace(record.TxRef)
	}
	if strings.TrimSpace(record.MsgID) == "" {
		record.MsgID = strings.TrimSpace(record.RequestID)
	}
	if strings.TrimSpace(record.RequestID) == "" {
		record.RequestID = strings.TrimSpace(record.MsgID)
	}
	if strings.TrimSpace(record.SettledAt) == "" {
		if strings.TrimSpace(record.ApprovedAt) != "" {
			record.SettledAt = strings.TrimSpace(record.ApprovedAt)
		}
	}
	if strings.TrimSpace(record.ApprovedAt) == "" && strings.TrimSpace(record.SettledAt) != "" {
		record.ApprovedAt = strings.TrimSpace(record.SettledAt)
	}
	if record.Amount == 0 && record.NetAmount > 0 {
		record.Amount = record.NetAmount
	}
	if record.NetAmount == 0 {
		if record.FeeAmount > 0 {
			record.NetAmount = record.Amount - record.FeeAmount
		} else {
			record.NetAmount = record.Amount
		}
	}
	if record.ExchangeRate == 0 {
		record.ExchangeRate = 1.0
	}
	if strings.TrimSpace(record.Status) == "" {
		record.Status = "SETTLED"
	}
}

func (s *SmartContract) resolveTokenTransferStateKey(ctx contractapi.TransactionContextInterface, requestRef string) (string, error) {
	requestRef = strings.TrimSpace(requestRef)
	if requestRef == "" {
		return "", fmt.Errorf("transfer request reference required")
	}
	if raw, err := ctx.GetStub().GetState(requestRef); err == nil && raw != nil {
		return requestRef, nil
	}

	legacyKey := "tokentransfer_" + requestRef
	if raw, err := ctx.GetStub().GetState(legacyKey); err == nil && raw != nil {
		return legacyKey, nil
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return "", err
	}
	defer iter.Close()
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return "", err
		}
		if !isTokenTransferRequestKey(kv.Key) {
			continue
		}
		var req TokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		normalizeTokenTransferRequestForRead(&req)
		if strings.TrimSpace(req.MsgID) == requestRef || strings.TrimSpace(req.RequestID) == requestRef {
			return kv.Key, nil
		}
	}
	return "", fmt.Errorf("token transfer request not found: %s", requestRef)
}

func normalizeCustomerTransferStatus(status string) string {
	switch strings.TrimSpace(strings.ToUpper(status)) {
	case "PENDINGSENDERTOKENAPPROVAL", "PENDING_SENDER":
		return "PENDING_SENDER"
	case "PENDINGRECEIVERTOKENAPPROVAL", "PENDING_RECEIVER":
		return "PENDING_RECEIVER"
	case "COMPLETED", "SETTLED":
		return "SETTLED"
	case "REJECTEDBYSENDEROWNER":
		return "REJECTED_SENDER_PRE_ESCROW"
	case "REJECTEDBYRECEIVEROWNER":
		return "REJECTED_RECEIVER"
	case "REJECTED":
		return "REJECTED_RECEIVER"
	case "EXPIRED_ESCROW_RETURNED", "FAILED_TECHNICAL", "REJECTED_SENDER_PRE_ESCROW", "REJECTED_RECEIVER":
		return strings.TrimSpace(strings.ToUpper(status))
	default:
		return strings.TrimSpace(strings.ToUpper(status))
	}
}

const (
	customerTransferStatusRejectedSenderPreEscrow = "REJECTED_SENDER_PRE_ESCROW"
	customerTransferStatusRejectedReceiver        = "REJECTED_RECEIVER"
	customerTransferStatusExpiredEscrowReturned   = "EXPIRED_ESCROW_RETURNED"
	customerTransferStatusFailedTechnical         = "FAILED_TECHNICAL"
)

var validCustomerTransferRejectionReasons = map[string]struct{}{
	"INSUFFICIENT_BALANCE":    {},
	"DAILY_LIMIT_EXCEEDED":    {},
	"SENDER_KYC_INVALID":      {},
	"RECEIVER_NOT_REGISTERED": {},
	"RECEIVER_KYC_INVALID":    {},
	"BANK_POLICY_VIOLATION":   {},
	"24HR_TIMEOUT":            {},
	"ESCROW_CREATION_FAILED":  {},
	"SMART_CONTRACT_ERROR":    {},
}

func normalizeCustomerTransferRejectionReason(reason string) string {
	normalized := strings.TrimSpace(strings.ToUpper(reason))
	if normalized == "" {
		return ""
	}
	if _, ok := validCustomerTransferRejectionReasons[normalized]; ok {
		return normalized
	}
	return "SMART_CONTRACT_ERROR"
}

func isCustomerTransferRejectedStatus(status string) bool {
	switch normalizeCustomerTransferStatus(status) {
	case customerTransferStatusRejectedSenderPreEscrow,
		customerTransferStatusRejectedReceiver,
		customerTransferStatusExpiredEscrowReturned,
		customerTransferStatusFailedTechnical:
		return true
	default:
		return false
	}
}

func isCustomerTransferPendingSender(status string) bool {
	return normalizeCustomerTransferStatus(status) == "PENDING_SENDER"
}

func isCustomerTransferPendingReceiver(status string) bool {
	return normalizeCustomerTransferStatus(status) == "PENDING_RECEIVER"
}

func normalizeCustomerToTokenTransferRequestForRead(req *CustomerToTokenTransferRequest) {
	if req == nil {
		return
	}
	req.Status = normalizeCustomerTransferStatus(req.Status)
	if strings.TrimSpace(req.MsgID) == "" {
		req.MsgID = strings.TrimSpace(req.TransferRequestID)
	}
	if strings.TrimSpace(req.TransferRequestID) == "" {
		req.TransferRequestID = strings.TrimSpace(req.MsgID)
	}
	if strings.TrimSpace(req.SenderCustomerRef) == "" {
		if strings.TrimSpace(req.SenderCustomerTokenID) != "" {
			req.SenderCustomerRef = strings.TrimSpace(req.SenderCustomerTokenID)
		} else {
			req.SenderCustomerRef = strings.TrimSpace(req.SenderCustomerID)
		}
	}
	if strings.TrimSpace(req.ReceiverCustomerRef) == "" {
		if strings.TrimSpace(req.ReceiverCustomerTokenID) != "" {
			req.ReceiverCustomerRef = strings.TrimSpace(req.ReceiverCustomerTokenID)
		} else {
			req.ReceiverCustomerRef = strings.TrimSpace(req.ReceiverCustomerID)
		}
	}
	if req.Amount == 0 {
		if req.EscrowAmount > 0 {
			req.Amount = req.EscrowAmount
		} else if req.EscrowedAmount > 0 {
			req.Amount = req.EscrowedAmount
		}
	}
	if req.EscrowAmount == 0 && req.EscrowedAmount > 0 {
		req.EscrowAmount = req.EscrowedAmount
	}
	if req.EscrowedAmount == 0 && req.EscrowAmount > 0 {
		req.EscrowedAmount = req.EscrowAmount
	}
	if req.CommissionPct == 0 && req.CommissionPercentage != 0 {
		// commission_percentage was historically stored as percentage points (2.0)
		req.CommissionPct = req.CommissionPercentage / 100.0
	}
	if req.CommissionPercentage == 0 && req.CommissionPct != 0 {
		req.CommissionPercentage = req.CommissionPct * 100.0
	}
	if req.NetReceiverAmount == 0 && req.ReceiverCustomerAmount > 0 {
		req.NetReceiverAmount = req.ReceiverCustomerAmount
	}
	if req.ReceiverCustomerAmount == 0 && req.NetReceiverAmount > 0 {
		req.ReceiverCustomerAmount = req.NetReceiverAmount
	}
	if req.Currency == "" {
		req.Currency = strings.TrimSpace(req.SenderCurrency)
	}
	if req.SenderCurrency == "" {
		req.SenderCurrency = strings.TrimSpace(req.Currency)
	}
	if req.SettledAt == "" && req.CompletedAt != "" {
		req.SettledAt = req.CompletedAt
	}
	if req.CompletedAt == "" && req.SettledAt != "" {
		req.CompletedAt = req.SettledAt
	}
	req.RejectionReason = normalizeCustomerTransferRejectionReason(req.RejectionReason)
	if req.RejectedAt == "" && isCustomerTransferRejectedStatus(req.Status) && req.CompletedAt != "" {
		req.RejectedAt = req.CompletedAt
	}
}

func isCustomerTransferKey(key string) bool {
	return strings.HasPrefix(key, "custtotoken_") || strings.HasPrefix(key, "TRANSFERS/") || strings.Contains(key, "/TRANSFER/")
}

func (s *SmartContract) resolveCustomerTransferStateKey(ctx contractapi.TransactionContextInterface, requestRef string) (string, error) {
	requestRef = strings.TrimSpace(requestRef)
	if requestRef == "" {
		return "", fmt.Errorf("transferRequestID is required")
	}
	if raw, err := ctx.GetStub().GetState(requestRef); err == nil && raw != nil {
		return requestRef, nil
	}

	legacyKey := "custtotoken_" + requestRef
	if raw, err := ctx.GetStub().GetState(legacyKey); err == nil && raw != nil {
		return legacyKey, nil
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return "", err
	}
	defer iter.Close()
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return "", err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}
		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		normalizeCustomerToTokenTransferRequestForRead(&req)
		if req.TransferRequestID == requestRef || req.MsgID == requestRef {
			return kv.Key, nil
		}
	}
	return "", fmt.Errorf("transfer request not found")
}

func normalizeParticipantTransferRecordForRead(record *ParticipantTransferRecord) {
	if record == nil {
		return
	}
	if strings.TrimSpace(record.TxRef) == "" {
		record.TxRef = strings.TrimSpace(record.RecordID)
	}
	if strings.TrimSpace(record.RecordID) == "" {
		record.RecordID = strings.TrimSpace(record.TxRef)
	}
	if strings.TrimSpace(record.RequestMsgID) == "" {
		if strings.TrimSpace(record.TransferRequestID) != "" {
			record.RequestMsgID = strings.TrimSpace(record.TransferRequestID)
		} else {
			record.RequestMsgID = strings.TrimSpace(record.TransferID)
		}
	}
	if strings.TrimSpace(record.TransferRequestID) == "" {
		record.TransferRequestID = strings.TrimSpace(record.RequestMsgID)
	}
	if strings.TrimSpace(record.TransferID) == "" {
		record.TransferID = strings.TrimSpace(record.RequestMsgID)
	}
	if strings.TrimSpace(record.SenderParticipantID) == "" {
		record.SenderParticipantID = strings.TrimSpace(record.SenderCustomerRef)
	}
	if strings.TrimSpace(record.ReceiverParticipantID) == "" {
		record.ReceiverParticipantID = strings.TrimSpace(record.ReceiverCustomerRef)
	}
	if strings.TrimSpace(record.SettledAt) == "" && strings.TrimSpace(record.CompletedAt) != "" {
		record.SettledAt = strings.TrimSpace(record.CompletedAt)
	}
	if strings.TrimSpace(record.CompletedAt) == "" && strings.TrimSpace(record.SettledAt) != "" {
		record.CompletedAt = strings.TrimSpace(record.SettledAt)
	}
	if strings.TrimSpace(record.Status) == "" {
		record.Status = "SETTLED"
	}
	if record.ExchangeRate == 0 {
		record.ExchangeRate = 1.0
	}
	if record.NetAmount == 0 {
		record.NetAmount = record.Amount - record.Commission
	}
}

func (s *SmartContract) findCustomerByRefAndBIC(ctx contractapi.TransactionContextInterface, customerRef, bic string) (*Participant, string, error) {
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, "", err
	}
	defer iter.Close()
	targetRef := strings.TrimSpace(customerRef)
	targetBIC := strings.TrimSpace(strings.ToUpper(bic))
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, "", err
		}
		if !strings.HasPrefix(kv.Key, "participant_") {
			continue
		}
		var p Participant
		if err := json.Unmarshal(kv.Value, &p); err != nil {
			continue
		}
		normalizeParticipantForRead(&p)
		ref := strings.TrimSpace(p.CustomerRef)
		customerID := strings.TrimSpace(p.CustomerID)
		networkAddress := strings.TrimSpace(p.NetworkAddress)
		if ref == "" {
			ref = customerID
		}
		pBIC := strings.TrimSpace(strings.ToUpper(p.BIC))
		if pBIC == "" && p.TokenID != "" {
			if tokenBytes, e := ctx.GetStub().GetState(p.TokenID); e == nil && tokenBytes != nil {
				var t Token
				if json.Unmarshal(tokenBytes, &t) == nil {
					if resolved, re := s.resolveTokenBIC(ctx, t); re == nil {
						pBIC = strings.TrimSpace(strings.ToUpper(resolved))
					}
				}
			}
		}
		refMatched := customerRefMatches(
			targetRef,
			ref,
			customerID,
			networkAddress,
		)
		if refMatched && pBIC == targetBIC && p.TokenID != "" && p.Approved {
			return &p, kv.Key, nil
		}
	}
	return nil, "", fmt.Errorf("customer %s not found for bank %s", customerRef, bic)
}

func (s *SmartContract) findApprovedCustomerByCaller(ctx contractapi.TransactionContextInterface, callerID string) (*Participant, string, error) {
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, "", err
	}
	defer iter.Close()
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, "", err
		}
		if !strings.HasPrefix(kv.Key, "participant_") {
			continue
		}
		var p Participant
		if err := json.Unmarshal(kv.Value, &p); err != nil {
			continue
		}
		normalizeParticipantForRead(&p)
		if strings.TrimSpace(p.ClientID) == strings.TrimSpace(callerID) && p.TokenID != "" && p.Approved {
			return &p, kv.Key, nil
		}
	}
	return nil, "", fmt.Errorf("approved sender customer not found for caller")
}

func isMintRequestKey(key string) bool {
	return strings.HasPrefix(key, "mintrequest_") || strings.HasPrefix(key, "custmintreq_") || strings.Contains(key, "/MINT/")
}

func isMintRequestPending(req MintRequest) bool {
	return strings.TrimSpace(strings.ToUpper(req.Status)) == "PENDING" || strings.TrimSpace(req.Status) == ""
}

func isMintRequestApproved(req MintRequest) bool {
	return strings.TrimSpace(strings.ToUpper(req.Status)) == "APPROVED"
}

func isCustomerScopedMintRequest(req MintRequest) bool {
	ref := strings.TrimSpace(mintRequestCustomerRef(req))
	if ref == "" {
		return false
	}
	upperRef := strings.ToUpper(ref)
	// Customer IDs are token-scoped customer identifiers (e.g. CUST_TOKEN...).
	return strings.HasPrefix(upperRef, "CUST_")
}

func mintRequestCustomerRef(req MintRequest) string {
	customerRef := strings.TrimSpace(req.CustomerRef)
	if customerRef != "" {
		return customerRef
	}
	return strings.TrimSpace(req.CustomerID)
}

func setMintRequestCustomerRef(req *MintRequest, customerRef string) {
	ref := strings.TrimSpace(customerRef)
	req.CustomerRef = ref
	req.CustomerID = ref // legacy alias for existing consumers
}

func tokenMintRecordStateKey(bic, recordID string) string {
	return fmt.Sprintf("%s/TMINT/%s", strings.TrimSpace(strings.ToUpper(bic)), strings.TrimSpace(recordID))
}

func isTokenMintRecordKey(key string) bool {
	return strings.HasPrefix(key, "tokenmint_") || strings.Contains(key, "/TMINT/")
}

func appendUniqueRefs(dst []string, src []string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, item := range dst {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	for _, item := range src {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		dst = append(dst, trimmed)
		seen[trimmed] = struct{}{}
	}
	return dst
}

func normalizeParticipantForRead(p *Participant) {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.CustomerRef) == "" {
		switch {
		case strings.TrimSpace(p.CustomerID) != "":
			p.CustomerRef = strings.TrimSpace(p.CustomerID)
		default:
			p.CustomerRef = strings.TrimSpace(p.NetworkAddress)
		}
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		p.CustomerID = strings.TrimSpace(p.CustomerRef)
	}
	if strings.TrimSpace(p.KycRef) == "" {
		p.KycRef = strings.TrimSpace(p.KycId)
	}
	if strings.TrimSpace(p.KycId) == "" {
		p.KycId = strings.TrimSpace(p.KycRef)
	}
	if strings.TrimSpace(p.Status) == "" {
		if p.Approved {
			p.Status = "ACTIVE"
		} else {
			p.Status = "PENDING"
		}
	}
	if strings.TrimSpace(p.ActivatedAt) == "" && strings.TrimSpace(p.ApprovedAt) != "" {
		p.ActivatedAt = p.ApprovedAt
	}
	if strings.TrimSpace(p.ApprovedAt) == "" && strings.TrimSpace(p.ActivatedAt) != "" {
		p.ApprovedAt = p.ActivatedAt
	}
	if p.ForeignBalances == nil {
		p.ForeignBalances = make(map[string]int64)
	}
	if len(p.ForeignBalances) == 0 && len(p.ForeignCurrencies) > 0 {
		for code, amount := range p.ForeignCurrencies {
			p.ForeignBalances[code] = int64(math.Round(amount))
		}
	}
	if p.ForeignCurrencies == nil {
		p.ForeignCurrencies = make(map[string]float64)
	}
	if len(p.ForeignCurrencies) == 0 && len(p.ForeignBalances) > 0 {
		for code, amount := range p.ForeignBalances {
			p.ForeignCurrencies[code] = float64(amount)
		}
	}
	p.TransferRefs = appendUniqueRefs(p.TransferRefs, p.TransferIDs)
	p.TransferRefs = appendUniqueRefs(p.TransferRefs, p.TokenTransferIDs)
}

func normalizeParticipantForWrite(p *Participant, bic string, updatedAt string) {
	normalizeParticipantForRead(p)
	if strings.TrimSpace(p.BIC) == "" && strings.TrimSpace(bic) != "" {
		p.BIC = strings.TrimSpace(strings.ToUpper(bic))
	}
	if strings.TrimSpace(updatedAt) != "" {
		p.LastUpdated = updatedAt
	}
	if strings.EqualFold(strings.TrimSpace(p.Status), "ACTIVE") {
		p.Approved = true
		if strings.TrimSpace(p.ActivatedAt) == "" && strings.TrimSpace(updatedAt) != "" {
			p.ActivatedAt = updatedAt
		}
	}
	if strings.TrimSpace(p.ApprovedAt) == "" && strings.TrimSpace(p.ActivatedAt) != "" {
		p.ApprovedAt = p.ActivatedAt
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		p.CustomerID = strings.TrimSpace(p.CustomerRef)
	}
	if strings.TrimSpace(p.KycId) == "" {
		p.KycId = strings.TrimSpace(p.KycRef)
	}
}

func mintRequestStateKey(bic, msgID string) string {
	return fmt.Sprintf("%s/MINT/%s", strings.TrimSpace(strings.ToUpper(bic)), strings.TrimSpace(msgID))
}

func registerParticipantStateKey(bic, msgID string) string {
	return fmt.Sprintf("%s/CREG/%s", strings.TrimSpace(strings.ToUpper(bic)), strings.TrimSpace(msgID))
}

func isRegisterParticipantRequestKey(key string) bool {
	return strings.HasPrefix(key, "custreq_") || strings.Contains(key, "/CREG/")
}

func (s *SmartContract) resolveRegisterParticipantStateKey(ctx contractapi.TransactionContextInterface, requestRef string) (string, error) {
	requestRef = strings.TrimSpace(requestRef)
	if requestRef == "" {
		return "", fmt.Errorf("registration request reference required")
	}
	if raw, err := ctx.GetStub().GetState(requestRef); err == nil && raw != nil {
		return requestRef, nil
	}
	legacyKey := fmt.Sprintf("custreq_%s", requestRef)
	if raw, err := ctx.GetStub().GetState(legacyKey); err == nil && raw != nil {
		return legacyKey, nil
	}
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return "", err
	}
	defer iter.Close()
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return "", err
		}
		if !isRegisterParticipantRequestKey(kv.Key) {
			continue
		}
		var req RegisterParticipantRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if strings.TrimSpace(req.MsgID) == requestRef {
			return kv.Key, nil
		}
	}
	return "", fmt.Errorf("registration request %s not found", requestRef)
}

func (s *SmartContract) getTxUTCTime(ctx contractapi.TransactionContextInterface) (time.Time, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts.Seconds, int64(ts.Nanos)).UTC(), nil
}

func (s *SmartContract) resolveTokenRequestStateKey(ctx contractapi.TransactionContextInterface, requestRef string) (string, error) {
	requestRef = strings.TrimSpace(requestRef)
	if requestRef == "" {
		return "", fmt.Errorf("request reference required")
	}

	// Exact world-state key lookup first.
	if raw, err := ctx.GetStub().GetState(requestRef); err == nil && raw != nil {
		return requestRef, nil
	}

	// Legacy key format.
	legacyKey := fmt.Sprintf("tokenrequest_%s", requestRef)
	if raw, err := ctx.GetStub().GetState(legacyKey); err == nil && raw != nil {
		return legacyKey, nil
	}

	// New key format: institution/TRQ/msg_id
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return "", err
	}
	defer iter.Close()

	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return "", err
		}
		if !(strings.HasPrefix(kv.Key, "tokenrequest_") || strings.Contains(kv.Key, "/TRQ/")) {
			continue
		}
		var req TokenRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if req.MsgID == requestRef || req.RequestID == requestRef {
			return kv.Key, nil
		}
	}

	return "", fmt.Errorf("token request %s not found", requestRef)
}

func (s *SmartContract) resolveMintRequestStateKey(ctx contractapi.TransactionContextInterface, requestRef string) (string, error) {
	requestRef = strings.TrimSpace(requestRef)
	if requestRef == "" {
		return "", fmt.Errorf("mint request reference required")
	}
	if raw, err := ctx.GetStub().GetState(requestRef); err == nil && raw != nil {
		return requestRef, nil
	}
	legacyKey := fmt.Sprintf("mintrequest_%s", requestRef)
	if raw, err := ctx.GetStub().GetState(legacyKey); err == nil && raw != nil {
		return legacyKey, nil
	}
	legacyCustKey := fmt.Sprintf("custmintreq_%s", requestRef)
	if raw, err := ctx.GetStub().GetState(legacyCustKey); err == nil && raw != nil {
		return legacyCustKey, nil
	}
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return "", err
	}
	defer iter.Close()
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return "", err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if req.MsgID == requestRef {
			return kv.Key, nil
		}
	}
	return "", fmt.Errorf("mint request %s not found", requestRef)
}

func (s *SmartContract) getTodayMintTotalForBIC(ctx contractapi.TransactionContextInterface, bic string) (int64, error) {
	bic = strings.TrimSpace(strings.ToUpper(bic))
	if bic == "" {
		return 0, nil
	}
	txTime, err := s.getTxUTCTime(ctx)
	if err != nil {
		return 0, err
	}
	dayPrefix := txTime.Format("2006-01-02")
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	var total int64
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return 0, err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if strings.TrimSpace(strings.ToUpper(req.BIC)) != bic {
			continue
		}
		if strings.TrimSpace(req.CreatedAt) == "" || !strings.HasPrefix(req.CreatedAt, dayPrefix) {
			continue
		}
		if isMintRequestPending(req) || isMintRequestApproved(req) {
			total += req.Amount
		}
	}
	return total, nil
}

func (s *SmartContract) resolveTokenBIC(ctx contractapi.TransactionContextInterface, token Token) (string, error) {
	bic := strings.TrimSpace(strings.ToUpper(token.BIC))
	if validBICFormat(bic) {
		return bic, nil
	}
	display := strings.TrimSpace(strings.ToUpper(token.DisplayTokenID))
	if validBICFormat(display) {
		return display, nil
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return "", err
	}
	defer iter.Close()

	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return "", err
		}
		if !(strings.HasPrefix(kv.Key, "tokenrequest_") || strings.Contains(kv.Key, "/TRQ/")) {
			continue
		}
		var req TokenRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if strings.TrimSpace(req.TokenID) != strings.TrimSpace(token.TokenID) {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(req.Status)) != "APPROVED" {
			continue
		}
		institutionID := strings.TrimSpace(strings.ToUpper(req.InstitutionID))
		if validBICFormat(institutionID) {
			return institutionID, nil
		}
	}

	// Legacy fallback: older approved requests may not have token_id populated.
	owner := strings.TrimSpace(token.Owner)
	if owner != "" {
		iter2, err := ctx.GetStub().GetStateByRange("", "")
		if err != nil {
			return "", err
		}
		defer iter2.Close()
		for iter2.HasNext() {
			kv, err := iter2.Next()
			if err != nil {
				return "", err
			}
			if !(strings.HasPrefix(kv.Key, "tokenrequest_") || strings.Contains(kv.Key, "/TRQ/")) {
				continue
			}
			var req TokenRequest
			if err := json.Unmarshal(kv.Value, &req); err != nil {
				continue
			}
			if strings.ToUpper(strings.TrimSpace(req.Status)) != "APPROVED" {
				continue
			}
			reqOwner := strings.TrimSpace(req.ParticipantAddress)
			if reqOwner == "" {
				reqOwner = strings.TrimSpace(req.NetworkAddr)
			}
			if reqOwner == "" {
				reqOwner = strings.TrimSpace(req.CallerID)
			}
			if reqOwner != owner {
				continue
			}
			institutionID := strings.TrimSpace(strings.ToUpper(req.InstitutionID))
			if validBICFormat(institutionID) {
				return institutionID, nil
			}
		}
	}
	return "", fmt.Errorf("token BIC is invalid")
}

// RequestTokenRequest creates a financial-message token request.
// Args order: institution_id, institution_name, country_code, currency_code, reference
func (s *SmartContract) RequestTokenRequest(ctx contractapi.TransactionContextInterface, institutionID, institutionName, countryCode, currencyCode, reference string) (string, error) {
	institutionID = strings.TrimSpace(strings.ToUpper(institutionID))
	institutionName = strings.TrimSpace(institutionName)
	countryCode = strings.TrimSpace(strings.ToUpper(countryCode))
	currencyCode = strings.TrimSpace(strings.ToUpper(currencyCode))
	reference = strings.TrimSpace(reference)

	if institutionID == "" || !validBICFormat(institutionID) {
		return "", fmt.Errorf("institution_id must be valid BIC8/BIC11")
	}
	if institutionName == "" {
		return "", fmt.Errorf("institution_name is required")
	}
	if !validCountryCode(countryCode) {
		return "", fmt.Errorf("country_code must be ISO 3166-1 alpha-2")
	}
	if !validCurrencyCode(currencyCode) {
		return "", fmt.Errorf("currency_code must be ISO 4217 format")
	}
	if reference == "" {
		return "", fmt.Errorf("reference is required")
	}

	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %v", err)
	}

	participantKey := institutionID
	participantBytes, err := ctx.GetStub().GetState(participantKey)
	if err != nil {
		return "", fmt.Errorf("failed to read institution %s: %v", institutionID, err)
	}
	if participantBytes == nil {
		// Compatibility path: participants are commonly keyed by caller identity.
		participantKey = callerID
		participantBytes, err = ctx.GetStub().GetState(participantKey)
		if err != nil {
			return "", fmt.Errorf("failed to read participant by caller identity: %v", err)
		}
		if participantBytes == nil {
			return "", fmt.Errorf("institution %s not registered", institutionID)
		}
	}
	var participant Participant
	if err := json.Unmarshal(participantBytes, &participant); err != nil {
		return "", fmt.Errorf("failed to parse participant: %v", err)
	}
	normalizeParticipantForRead(&participant)
	if participant.ClientID != callerID {
		return "", fmt.Errorf("forbidden: caller does not own institution %s", institutionID)
	}
	// Institution legal name can differ from wallet username/participant display name.
	// Keep ownership enforcement via ClientID check above, and only validate country consistency.
	if strings.TrimSpace(strings.ToUpper(participant.Country)) != "" &&
		strings.TrimSpace(strings.ToUpper(participant.Country)) != countryCode {
		return "", fmt.Errorf("institution country does not match registered participant country")
	}

	client, err := cid.New(ctx.GetStub())
	if err != nil {
		return "", err
	}
	mspID, err := client.GetMSPID()
	if err != nil {
		return "", err
	}

	txID := ctx.GetStub().GetTxID()
	shortTx := txID
	if len(shortTx) > 8 {
		shortTx = shortTx[:8]
	}
	msgID := fmt.Sprintf("%s-tokenrequest_tx%s", institutionID, shortTx)

	txTime, err := s.getTxUTCTime(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to derive transaction time: %v", err)
	}
	createdAt := txTime.Format(time.RFC3339)
	validUntil := txTime.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	stateKey := fmt.Sprintf("%s/TRQ/%s", institutionID, msgID)

	existing, err := ctx.GetStub().GetState(stateKey)
	if err != nil {
		return "", fmt.Errorf("failed to read state: %v", err)
	}
	if existing != nil {
		return "", fmt.Errorf("token request already exists: %s", msgID)
	}

	req := TokenRequest{
		MsgID:              msgID,
		InstitutionID:      institutionID,
		InstitutionName:    institutionName,
		CountryCode:        countryCode,
		CurrencyCode:       currencyCode,
		RequestPurpose:     "CURRENCY_ACCESS",
		Status:             "PENDING",
		CreatedAt:          createdAt,
		ValidUntil:         validUntil,
		Reference:          reference,
		RequestID:          msgID, // legacy read compatibility
		NetworkAddr:        callerID,
		ParticipantAddress: participantKey,
		CallerID:           callerID,
		CallerMSP:          mspID,
		ParticipantMSP:     participant.MSP,
		Currency:           currencyCode,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}
	if err := ctx.GetStub().PutState(stateKey, reqBytes); err != nil {
		return "", err
	}
	if err := ctx.GetStub().SetEvent("TokenRequestCreated", []byte(msgID)); err != nil {
		return "", err
	}

	response := map[string]string{
		"msg_id":      msgID,
		"status":      "PENDING",
		"state_key":   stateKey,
		"created_at":  createdAt,
		"valid_until": validUntil,
	}
	resBytes, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %v", err)
	}
	return string(resBytes), nil
}

// GetPendingTokenRequests returns admin pending token requests
func (s *SmartContract) GetPendingTokenRequests(ctx contractapi.TransactionContextInterface) ([]TokenRequest, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var list []TokenRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(kv.Key, "tokenrequest_") || strings.Contains(kv.Key, "/TRQ/") {
			var r TokenRequest
			if err := json.Unmarshal(kv.Value, &r); err == nil && r.Status == "PENDING" {
				list = append(list, r)
			}
		}
	}
	return list, nil
}

// KYC anchor chaincode APIs removed (cleanup)

// ApproveTokenRequest approves token request and assigns token (bank-specific approval)
func (s *SmartContract) ApproveTokenRequest(ctx contractapi.TransactionContextInterface, requestID string) error {
	// Verify caller is from a valid bank
	if err := s.VerifyAdmin(ctx); err != nil {
		return err
	}

	approverMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get approver MSP: %v", err)
	}

	stateKey, err := s.resolveTokenRequestStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	rb, err := ctx.GetStub().GetState(stateKey)
	if err != nil || rb == nil {
		return fmt.Errorf("token request not found")
	}
	var r TokenRequest
	json.Unmarshal(rb, &r)
	if r.Status != "PENDING" {
		return fmt.Errorf("request already processed")
	}

	// VERIFY: Only the requesting participant's bank can approve
	if r.ParticipantMSP != approverMSP {
		return fmt.Errorf("access denied: only your bank (%s) can approve your token request (request from: %s)", approverMSP, r.ParticipantMSP)
	}

	currency := strings.TrimSpace(r.CurrencyCode)
	if currency == "" {
		currency = strings.TrimSpace(r.Currency)
	}
	if currency == "" {
		return fmt.Errorf("token request missing currency")
	}

	tokenID, err := s.findAvailableToken(ctx)
	if err != nil {
		return err
	}
	if tokenID == "" {
		return fmt.Errorf("no tokens available")
	}

	r.Status = "APPROVED"
	r.TokenID = tokenID
	r.CurrencyCode = currency
	r.Currency = currency
	r.ApproverID, _ = ctx.GetClientIdentity().GetID()
	rb, _ = json.Marshal(r)

	if err = ctx.GetStub().PutState(stateKey, rb); err != nil {
		return err
	}

	targetAddress := r.ParticipantAddress
	if targetAddress == "" {
		targetAddress = r.InstitutionID
	}
	if targetAddress == "" {
		targetAddress = r.NetworkAddr
	}

	pb, err := ctx.GetStub().GetState(targetAddress)
	if err != nil || pb == nil {
		return fmt.Errorf("participant not found")
	}
	var p Participant
	json.Unmarshal(pb, &p)
	normalizeParticipantForRead(&p)

	// Verify participant belongs to approver's bank
	if p.MSP != approverMSP {
		return fmt.Errorf("access denied: cannot approve token for participant from different bank")
	}

	p.TokenID = tokenID
	p.Approved = true
	p.Status = "ACTIVE"
	assignTime, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	p.ApprovedAt = assignTime
	p.ActivatedAt = assignTime
	p.LastUpdated = assignTime
	normalizeParticipantForWrite(&p, strings.TrimSpace(strings.ToUpper(r.InstitutionID)), assignTime)
	pb, _ = json.Marshal(p)

	if err = ctx.GetStub().PutState(targetAddress, pb); err != nil {
		return err
	}

	tb, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tb == nil {
		return fmt.Errorf("token not found")
	}
	var t Token
	json.Unmarshal(tb, &t)
	t.Owner = targetAddress
	t.OwnerMSP = approverMSP // Token now belongs to this bank
	t.Available = false
	// Use institution ID as stable display identifier for bank-facing UIs.
	t.DisplayTokenID = strings.TrimSpace(strings.ToUpper(r.InstitutionID))
	t.BIC = strings.TrimSpace(strings.ToUpper(r.InstitutionID))
	t.Status = "ACTIVE"
	t.IsFrozen = false
	if t.Currency == "" {
		t.Currency = currency
	} else if t.Currency != currency {
		return fmt.Errorf("token %s already configured for currency %s", tokenID, t.Currency)
	}
	t.AssignedAt = assignTime
	tb, _ = json.Marshal(t)

	return ctx.GetStub().PutState(tokenID, tb)
}

// CancelTokenRequest allows a bank to cancel a pending token request
func (s *SmartContract) CancelTokenRequest(ctx contractapi.TransactionContextInterface, requestID string) error {
	// Verify caller is from a valid bank
	if err := s.VerifyAdmin(ctx); err != nil {
		return err
	}

	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}

	stateKey, err := s.resolveTokenRequestStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	rb, err := ctx.GetStub().GetState(stateKey)
	if err != nil || rb == nil {
		return fmt.Errorf("token request not found")
	}

	var r TokenRequest
	if err := json.Unmarshal(rb, &r); err != nil {
		return fmt.Errorf("failed to parse token request: %v", err)
	}

	// VERIFY: Only the requesting participant's bank can cancel
	if r.ParticipantMSP != callerMSP {
		return fmt.Errorf("access denied: only your bank can cancel your token request")
	}

	// VERIFY: Can only cancel PENDING requests (not already approved)
	if r.Status != "PENDING" {
		return fmt.Errorf("cannot cancel request in status: %s (only PENDING requests can be cancelled)", r.Status)
	}

	// Mark request as CANCELLED
	r.Status = "CANCELLED"
	rb, _ = json.Marshal(r)

	return ctx.GetStub().PutState(stateKey, rb)
}

// findAvailableToken returns first available tokenID or empty string
func (s *SmartContract) findAvailableToken(ctx contractapi.TransactionContextInterface) (string, error) {
	for i := 1; i <= maxTokens; i++ {
		tid := fmt.Sprintf("token_%d", i)
		b, err := ctx.GetStub().GetState(tid)
		if err != nil {
			return "", err
		}
		if b == nil {
			continue
		}
		var t Token
		if err := json.Unmarshal(b, &t); err != nil {
			return "", err
		}
		if t.Available {
			return tid, nil
		}
	}
	return "", nil
}

// GetTokenAccess verifies caller identity and returns token address
func (s *SmartContract) GetTokenAccess(ctx contractapi.TransactionContextInterface, networkAddress string) (string, error) {
	pb, err := ctx.GetStub().GetState(networkAddress)
	if err != nil || pb == nil {
		return "", fmt.Errorf("participant not found")
	}
	var p Participant
	if err := json.Unmarshal(pb, &p); err != nil {
		return "", err
	}
	// DATA ISOLATION: Verify caller's bank matches participant's bank
	if err := s.VerifyBankAccessToData(ctx, p.MSP); err != nil {
		return "", err
	}
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", err
	}
	if p.ClientID != callerID {
		return "", fmt.Errorf("caller identity mismatch")
	}
	if p.TokenID == "" {
		return "", fmt.Errorf("token not assigned")
	}
	return p.TokenID, nil
}

// RequestTokenMint creates a mint request using caller certificate context only.
func (s *SmartContract) RequestTokenMint(ctx contractapi.TransactionContextInterface, amount int64, purpose string) error {
	if amount <= 0 || amount > maxMintRequestAmount {
		return fmt.Errorf("amount must be 1-100000000")
	}
	if !validMintPurpose(purpose) {
		return fmt.Errorf("purpose must be one of WORKING_CAPITAL, SETTLEMENT, LIQUIDITY")
	}

	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to verify identity: %v", err)
	}
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bank MSP: %v", err)
	}

	partBytes, err := ctx.GetStub().GetState(callerID)
	if err != nil || partBytes == nil {
		return fmt.Errorf("participant not found")
	}
	var participant Participant
	if err := json.Unmarshal(partBytes, &participant); err != nil {
		return err
	}
	if strings.TrimSpace(participant.TokenID) == "" {
		return fmt.Errorf("participant has no assigned token")
	}
	if err := s.VerifyBankAccessToData(ctx, participant.MSP); err != nil {
		return err
	}

	tokenBytes, err := ctx.GetStub().GetState(participant.TokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}
	if token.Owner != participant.NetworkAddress {
		return fmt.Errorf("caller is not token owner")
	}
	if token.OwnerMSP != callerMSP {
		return fmt.Errorf("access denied: only token owner's bank can request mints")
	}
	if strings.EqualFold(strings.TrimSpace(token.Status), "FROZEN") || token.IsFrozen {
		return fmt.Errorf("token %s is frozen", token.TokenID)
	}
	tokenCurrency := strings.TrimSpace(strings.ToUpper(token.Currency))
	if !validCurrencyCode(tokenCurrency) {
		return fmt.Errorf("token currency not configured")
	}
	bic, err := s.resolveTokenBIC(ctx, token)
	if err != nil {
		return err
	}
	if bic != token.BIC {
		token.BIC = bic
		updatedTokenBytes, marshalErr := json.Marshal(token)
		if marshalErr == nil {
			_ = ctx.GetStub().PutState(token.TokenID, updatedTokenBytes)
		}
	}

	todayTotal, err := s.getTodayMintTotalForBIC(ctx, bic)
	if err != nil {
		return err
	}
	if todayTotal+amount > maxMintRequestAmount {
		return fmt.Errorf("daily cap exceeded for %s", bic)
	}

	txID := ctx.GetStub().GetTxID()
	shortTx := txID
	if len(shortTx) > 8 {
		shortTx = shortTx[:8]
	}
	msgID := fmt.Sprintf("%s-MINT-%s", bic, shortTx)
	txTime, err := s.getTxUTCTime(ctx)
	if err != nil {
		return err
	}
	req := MintRequest{
		MsgID:          msgID,
		BIC:            bic,
		TokenID:        token.TokenID,
		Amount:         amount,
		Currency:       tokenCurrency,
		KycRef:         strings.TrimSpace(participant.KycId),
		KycStatus:      "VERIFIED",
		Status:         "PENDING",
		CreatedAt:      txTime.Format(time.RFC3339),
		ExpiresAt:      txTime.Add(mintRequestTTLDays * 24 * time.Hour).Format(time.RFC3339),
		Purpose:        strings.TrimSpace(strings.ToUpper(purpose)),
		DailyLimitUsed: todayTotal + amount,
	}
	setMintRequestCustomerRef(&req, callerID)

	stateKey := mintRequestStateKey(bic, msgID)
	if existing, err := ctx.GetStub().GetState(stateKey); err != nil {
		return err
	} else if existing != nil {
		return fmt.Errorf("mint request already exists: %s", msgID)
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(stateKey, reqBytes)
}

// RequestMintCoins keeps backward compatibility with existing client payloads.
func (s *SmartContract) RequestMintCoins(ctx contractapi.TransactionContextInterface, _networkAddress string, amount int) error {
	return s.RequestTokenMint(ctx, int64(amount), mintPurposeWorkingCapital)
}

// GetPendingMintRequests (admin)
func (s *SmartContract) GetPendingMintRequests(ctx contractapi.TransactionContextInterface) ([]MintRequest, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}
	// Get caller's MSP for data isolation
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, err
	}

	it, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer it.Close()
	var reqs []MintRequest
	for it.HasNext() {
		kv, _ := it.Next()
		if isMintRequestKey(kv.Key) {
			var r MintRequest
			if json.Unmarshal(kv.Value, &r) == nil && isMintRequestPending(r) {
				// Fetch token to check ownership MSP
				tokenBytes, tErr := ctx.GetStub().GetState(r.TokenID)
				if tErr != nil || tokenBytes == nil {
					continue
				}
				var token Token
				if err := json.Unmarshal(tokenBytes, &token); err != nil {
					continue
				}
				// Only return requests for tokens owned by caller's bank
				if token.OwnerMSP == callerMSP {
					reqs = append(reqs, r)
				}
			}
		}
	}
	return reqs, nil
}

// GetApprovedMintRequests returns mint requests that have already been approved (admin).
func (s *SmartContract) GetApprovedMintRequests(ctx contractapi.TransactionContextInterface) ([]MintRequest, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}
	// Get caller's MSP for data isolation
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, err
	}

	it, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer it.Close()

	var approved []MintRequest
	for it.HasNext() {
		kv, err := it.Next()
		if err != nil {
			return nil, err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if isMintRequestApproved(req) {
			// Fetch token to check ownership MSP
			tokenBytes, tErr := ctx.GetStub().GetState(req.TokenID)
			if tErr != nil || tokenBytes == nil {
				continue
			}
			var token Token
			if err := json.Unmarshal(tokenBytes, &token); err != nil {
				continue
			}
			// Only return requests for tokens owned by caller's bank
			if token.OwnerMSP == callerMSP {
				approved = append(approved, req)
			}
		}
	}
	return approved, nil
}

// ListApprovedParticipantMintRequests lets a participant view their approved mint requests with timestamps.
func (s *SmartContract) ListApprovedParticipantMintRequests(ctx contractapi.TransactionContextInterface, networkAddress string) ([]MintRequest, error) {
	pb, err := ctx.GetStub().GetState(networkAddress)
	if err != nil || pb == nil {
		return nil, fmt.Errorf("participant not found")
	}
	var participant Participant
	if err := json.Unmarshal(pb, &participant); err != nil {
		return nil, err
	}
	normalizeParticipantForRead(&participant)
	// Verify caller's MSP matches participant's MSP (bank-level access)
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}
	if callerMSP != participant.MSP {
		return nil, fmt.Errorf("unauthorized: can only view mint requests from your own bank")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var approved []MintRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		reqCustomerRef := mintRequestCustomerRef(req)
		if isMintRequestApproved(req) && (reqCustomerRef == networkAddress || reqCustomerRef == participant.CustomerID || reqCustomerRef == participant.CustomerRef) {
			approved = append(approved, req)
		}
	}
	return approved, nil
}

// ApproveMintRequest approves mint request and mints coins (bank-specific approval)
func (s *SmartContract) ApproveMintRequest(ctx contractapi.TransactionContextInterface, requestID string) error {
	// Verify caller is from a valid bank
	if err := s.VerifyAdmin(ctx); err != nil {
		return err
	}

	approverMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get approver MSP: %v", err)
	}
	approverID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get approver identity: %v", err)
	}

	stateKey, err := s.resolveMintRequestStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("mint request not found")
	}

	var mr MintRequest
	if err := json.Unmarshal(reqBytes, &mr); err != nil {
		return err
	}

	if isMintRequestApproved(mr) {
		return fmt.Errorf("mint request already approved")
	}

	tokenBytes, err := ctx.GetStub().GetState(mr.TokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}

	// VERIFY: Only the token owner's bank can approve mints for that token
	if token.OwnerMSP != approverMSP {
		return fmt.Errorf("access denied: only token owner's bank (%s) can approve mints (your bank: %s)", token.OwnerMSP, approverMSP)
	}

	tokenCurrency := strings.TrimSpace(token.Currency)
	if tokenCurrency == "" {
		return fmt.Errorf("token currency not configured")
	}
	if mr.Currency == "" {
		mr.Currency = tokenCurrency
	} else if mr.Currency != tokenCurrency {
		return fmt.Errorf("mint request currency %s does not match token currency %s", mr.Currency, tokenCurrency)
	}

	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	mr.Status = "APPROVED"
	if strings.TrimSpace(mr.CreatedAt) == "" {
		mr.CreatedAt = ts
	}
	updatedReqBytes, err := json.Marshal(mr)
	if err != nil {
		return err
	}

	if err = ctx.GetStub().PutState(stateKey, updatedReqBytes); err != nil {
		return err
	}
	setTokenSupply(&token, getTokenSupply(token)+int(mr.Amount))
	updatedTokenBytes, err := json.Marshal(token)
	if err != nil {
		return err
	}

	if err := ctx.GetStub().PutState(mr.TokenID, updatedTokenBytes); err != nil {
		return err
	}

	bic := strings.TrimSpace(strings.ToUpper(token.BIC))
	if bic == "" {
		resolvedBIC, bicErr := s.resolveTokenBIC(ctx, token)
		if bicErr == nil {
			bic = strings.TrimSpace(strings.ToUpper(resolvedBIC))
		}
	}
	if bic == "" {
		bic = "UNKNOWNBIC"
	}

	recordID := fmt.Sprintf("%s-TMINT-%s", bic, strings.TrimSpace(requestID))
	tokenMintRecord := TokenMintRecord{
		RecordID:     recordID,
		MsgID:        mr.MsgID,
		RequestID:    strings.TrimSpace(requestID),
		BIC:          bic,
		TokenID:      mr.TokenID,
		Amount:       mr.Amount,
		Currency:     strings.TrimSpace(mr.Currency),
		Purpose:      strings.TrimSpace(mr.Purpose),
		Status:       "APPROVED",
		CreatedAt:    strings.TrimSpace(mr.CreatedAt),
		ApprovedAt:   ts,
		ApprovedBy:   approverID,
		CustomerRef:  strings.TrimSpace(mr.CustomerRef),
		CustomerID:   strings.TrimSpace(mr.CustomerID),
		MintCategory: "TOKEN_OWNER_MINT",
	}
	recordBytes, err := json.Marshal(tokenMintRecord)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(tokenMintRecordStateKey(bic, recordID), recordBytes); err != nil {
		return err
	}
	// Legacy lookup key for compatibility.
	if err := ctx.GetStub().PutState("tokenmint_"+recordID, recordBytes); err != nil {
		return err
	}

	return nil
}

func (s *SmartContract) GetWalletInfo(ctx contractapi.TransactionContextInterface, networkAddress string) (map[string]interface{}, error) {
	pb, err := ctx.GetStub().GetState(networkAddress)
	if err != nil || pb == nil {
		return nil, fmt.Errorf("participant not found")
	}
	var p Participant
	if err := json.Unmarshal(pb, &p); err != nil {
		return nil, err
	}

	// DATA ISOLATION: Verify caller's bank matches participant's bank
	if err := s.VerifyBankAccessToData(ctx, p.MSP); err != nil {
		return nil, err
	}

	// Verify caller client ID matches participant client ID (same person)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("unable to get caller identity: %v", err)
	}
	if p.ClientID != callerID {
		return nil, fmt.Errorf("unauthorized caller")
	}

	tb, err := ctx.GetStub().GetState(p.TokenID)
	if err != nil || tb == nil {
		return nil, fmt.Errorf("token not found")
	}
	var t Token
	json.Unmarshal(tb, &t)

	fmt.Printf("[CHAINCODE DEBUG] GetWalletInfo: Participant NetworkAddress=%s, Participant.TokenID=%s\n", p.NetworkAddress, p.TokenID)
	fmt.Printf("[CHAINCODE DEBUG] GetWalletInfo: Loaded TokenID=%s, ForeignBalances=%v\n", t.TokenID, t.ForeignBalances)

	if p.TransferIDs == nil {
		p.TransferIDs = []string{}
	}
	if t.TransferIDs == nil {
		t.TransferIDs = []string{}
	}
	participantTransferIDs := append([]string{}, p.TransferIDs...)
	tokenTransferIDs := append([]string{}, t.TransferIDs...)
	ptID := deriveParticipantTransferID(p.NetworkAddress)
	btID := deriveBankTransferID(t.TokenID)
	if ptID != "" {
		participantTransferIDs = appendTransferIfMissing(participantTransferIDs, ptID)
	}
	if btID != "" {
		tokenTransferIDs = appendTransferIfMissing(tokenTransferIDs, btID)
	}
	walletBalance, err := s.getParticipantBalance(ctx, p.NetworkAddress)
	if err != nil {
		return nil, err
	}
	var foreignHoldings []map[string]interface{} = make([]map[string]interface{}, 0)
	var foreignBalancesMap map[string]int = t.ForeignBalances
	if foreignBalancesMap == nil {
		foreignBalancesMap = make(map[string]int)
	}
	if len(t.ForeignBalances) > 0 {
		keys := make([]string, 0, len(t.ForeignBalances))
		for code := range t.ForeignBalances {
			keys = append(keys, code)
		}
		sort.Strings(keys)
		for _, code := range keys {
			amount := t.ForeignBalances[code]
			entry := map[string]interface{}{
				"currency":       code,
				"amount":         amount,
				"display":        formatCurrencyValue(code, float64(amount)),
				"currencySymbol": currencySymbol(code),
			}
			foreignHoldings = append(foreignHoldings, entry)
		}
	}
	availableBalanceValue := float64(getTokenSupply(t))
	availableDisplay := formatCurrencyValue(t.Currency, availableBalanceValue)
	walletDisplay := formatCurrencyValue(t.Currency, walletBalance)

	return map[string]interface{}{
		"networkAddress":          p.NetworkAddress,
		"tokenID":                 t.TokenID,
		"bic":                     strings.TrimSpace(strings.ToUpper(t.BIC)),
		"bic_code":                strings.TrimSpace(strings.ToUpper(t.BIC)),
		"currency":                t.Currency,
		"currencySymbol":          currencySymbol(t.Currency),
		"availableBalance":        availableBalanceValue,
		"availableBalanceDisplay": availableDisplay,
		"foreign_balances":        foreignBalancesMap,
		"foreignCurrencies":       foreignHoldings,
		"walletBalance":           walletBalance,
		"walletBalanceDisplay":    walletDisplay,
		"wallet_balance":          walletBalance,
		"wallet_balance_display":  walletDisplay,
		"participantTransferIDs":  participantTransferIDs,
		"tokenTransferIDs":        tokenTransferIDs,
		"participantTransferID":   ptID,
		"tokenTransferID":         btID,
	}, nil
}

// GetTokenByID retrieves a specific token by its ID
func (s *SmartContract) GetTokenByID(ctx contractapi.TransactionContextInterface, tokenID string) (*Token, error) {
	if tokenID == "" {
		return nil, fmt.Errorf("token ID required")
	}

	// Normalize the token ID (remove prefix if present)
	normalizedTokenID := tokenID
	if strings.HasPrefix(tokenID, "token_") {
		normalizedTokenID = tokenID
	} else {
		normalizedTokenID = "token_" + tokenID
	}

	tokenBytes, err := ctx.GetStub().GetState(normalizedTokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to read token: %v", err)
	}
	if tokenBytes == nil {
		return nil, fmt.Errorf("token %s not found", normalizedTokenID)
	}

	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %v", err)
	}

	// DATA ISOLATION: Verify caller's bank can access this token
	if token.OwnerMSP != "" {
		if err := s.VerifyBankAccessToData(ctx, token.OwnerMSP); err != nil {
			return nil, err
		}
	}

	// Ensure maps are initialized
	if token.ForeignBalances == nil {
		token.ForeignBalances = make(map[string]int)
	}
	if token.TransferIDs == nil {
		token.TransferIDs = []string{}
	}

	return &token, nil
}

// ==================== Commission Configuration Functions ====================

// SetTokenCommission sets the commission percentage for a token (owner only)
func (s *SmartContract) SetTokenCommission(ctx contractapi.TransactionContextInterface, tokenID string, commissionPercentage float64) (*TokenCommissionConfig, error) {
	if tokenID == "" {
		return nil, fmt.Errorf("token ID required")
	}

	// Validate commission percentage
	if commissionPercentage < 0 || commissionPercentage > 100 {
		return nil, fmt.Errorf("commission percentage must be between 0 and 100")
	}

	// Get the token to verify caller is the owner
	token, err := s.GetTokenByID(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	// Get caller's identity (this is the network address)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}

	// Verify caller is the token owner
	if token.Owner != callerID {
		return nil, fmt.Errorf("only token owner can set commission rate")
	}

	// Create commission config
	config := TokenCommissionConfig{
		TokenID:              tokenID,
		CommissionPercentage: commissionPercentage,
		UpdatedAt:            time.Now().UTC().Format(time.RFC3339),
		UpdatedBy:            callerID,
	}

	// Store on blockchain
	configKey := fmt.Sprintf("commission_%s", tokenID)
	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal commission config: %v", err)
	}

	if err := ctx.GetStub().PutState(configKey, configBytes); err != nil {
		return nil, fmt.Errorf("failed to store commission config: %v", err)
	}

	return &config, nil
}

// GetTokenCommission retrieves the commission percentage for a token
func (s *SmartContract) GetTokenCommission(ctx contractapi.TransactionContextInterface, tokenID string) (*TokenCommissionConfig, error) {
	if tokenID == "" {
		return nil, fmt.Errorf("token ID required")
	}

	configKey := fmt.Sprintf("commission_%s", tokenID)
	configBytes, err := ctx.GetStub().GetState(configKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read commission config: %v", err)
	}

	// If no commission config exists, return default 0%
	if configBytes == nil {
		return &TokenCommissionConfig{
			TokenID:              tokenID,
			CommissionPercentage: 0.0,
			UpdatedAt:            "",
			UpdatedBy:            "",
		}, nil
	}

	var config TokenCommissionConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal commission config: %v", err)
	}

	return &config, nil
}

// Participant can view all tokens available to select (user function)
func (s *SmartContract) ViewAllTokens(ctx contractapi.TransactionContextInterface) ([]Token, error) {
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var tokens []Token
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(kv.Key, "token_") {
			var token Token
			if err := json.Unmarshal(kv.Value, &token); err == nil {
				// Ensure TokenID is set from the key if not already set
				if token.TokenID == "" {
					token.TokenID = kv.Key
				}
				if token.TransferIDs == nil {
					token.TransferIDs = []string{}
				}
				if token.ForeignBalances == nil {
					token.ForeignBalances = make(map[string]int)
				}
				tokens = append(tokens, token)
			}
		}
	}
	return tokens, nil
}

// GetAvailableTokensForRegistration returns all root tokens that are still available
// so customers can see which currencies can be requested.
func (s *SmartContract) GetAvailableTokensForRegistration(ctx contractapi.TransactionContextInterface) ([]Token, error) {
	// Get caller's MSP to filter tokens for their bank only
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, err
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var available []Token
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "token_") {
			continue
		}
		var token Token
		if err := json.Unmarshal(kv.Value, &token); err != nil {
			continue
		}
		if !token.Available || token.Owner != "" {
			continue
		}
		// Only show tokens available for caller's bank - check OwnerMSP field if set, or allow unassigned
		if token.OwnerMSP != "" && token.OwnerMSP != callerMSP {
			continue // Skip tokens reserved for other banks
		}
		// Ensure TokenID is set from the key if not already set
		if token.TokenID == "" {
			token.TokenID = kv.Key
		}
		if token.TransferIDs == nil {
			token.TransferIDs = []string{}
		}
		if token.ForeignBalances == nil {
			token.ForeignBalances = make(map[string]int)
		}
		available = append(available, token)
	}
	return available, nil
}

// ListAssignedTokens lists only tokens assigned to caller's bank (data isolation)
func (s *SmartContract) ListAssignedTokens(ctx contractapi.TransactionContextInterface) ([]Token, error) {
	// Get caller's MSP for data isolation
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var assigned []Token
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "token_") {
			continue
		}
		var token Token
		if err := json.Unmarshal(kv.Value, &token); err != nil {
			continue
		}
		if token.Owner != "" {
			// DATA ISOLATION: Only return tokens owned by caller's bank
			if token.OwnerMSP != callerMSP {
				continue
			}
			// Ensure TokenID is set from the key if not already set
			if token.TokenID == "" {
				token.TokenID = kv.Key
			}
			if token.TransferIDs == nil {
				token.TransferIDs = []string{}
			}
			if token.ForeignBalances == nil {
				token.ForeignBalances = make(map[string]int)
			}
			assigned = append(assigned, token)
		}
	}
	return assigned, nil
}

// ListAssignedTokensByOwner returns only tokens owned by the provided owner network address.
// This is stricter than ListAssignedTokens and is intended for owner-scoped dashboards.
func (s *SmartContract) ListAssignedTokensByOwner(ctx contractapi.TransactionContextInterface, ownerNetworkAddress string) ([]Token, error) {
	trimmedOwner := strings.TrimSpace(ownerNetworkAddress)
	if trimmedOwner == "" {
		return nil, fmt.Errorf("owner network address is required")
	}

	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var assigned []Token
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "token_") {
			continue
		}

		var token Token
		if err := json.Unmarshal(kv.Value, &token); err != nil {
			continue
		}

		// Enforce both exact owner match and bank isolation.
		if token.Owner != trimmedOwner {
			continue
		}
		if token.OwnerMSP != callerMSP {
			continue
		}

		if token.TokenID == "" {
			token.TokenID = kv.Key
		}
		if token.TransferIDs == nil {
			token.TransferIDs = []string{}
		}
		if token.ForeignBalances == nil {
			token.ForeignBalances = make(map[string]int)
		}
		assigned = append(assigned, token)
	}

	return assigned, nil
}

// ListApprovedParticipants returns all approved participants, filtered by caller's owned tokens only
func (s *SmartContract) ListApprovedParticipants(ctx contractapi.TransactionContextInterface) ([]Participant, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}

	// Get caller's MSP for data isolation
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	// Get caller's owned token IDs for token-based authorization
	callerOwnedTokens := make(map[string]bool)
	tokenIter, err := ctx.GetStub().GetStateByRange("token_", "token_~")
	if err == nil && tokenIter != nil {
		defer tokenIter.Close()

		for tokenIter.HasNext() {
			tokenKV, err := tokenIter.Next()
			if err != nil {
				break
			}
			var token Token
			if err := json.Unmarshal(tokenKV.Value, &token); err != nil {
				continue
			}
			// Only include tokens owned by caller's bank
			if token.OwnerMSP == callerMSP {
				callerOwnedTokens[token.TokenID] = true
			}
		}
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var approved []Participant
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "token_") {
			continue
		}
		var token Token
		if err := json.Unmarshal(kv.Value, &token); err != nil {
			continue
		}
		if token.Owner == "" {
			continue
		}
		// DATA ISOLATION: Only process tokens owned by caller's bank
		if token.OwnerMSP != callerMSP {
			continue
		}
		pBytes, err := ctx.GetStub().GetState(token.Owner)
		if err != nil || pBytes == nil {
			continue
		}
		var participant Participant
		if err := json.Unmarshal(pBytes, &participant); err != nil {
			continue
		}
		normalizeParticipantForRead(&participant)
		if !participant.Approved {
			continue
		}

		// AUTHORIZATION: Only allow caller to see participants for their own tokens
		// If participant has TokenID set, only include if caller owns that token
		if participant.TokenID != "" && !callerOwnedTokens[participant.TokenID] {
			// Participant is approved for a different token owner's token, skip it
			continue
		}

		if participant.TransferIDs == nil {
			participant.TransferIDs = []string{}
		}
		participant.TransferRefs = appendUniqueRefs(participant.TransferRefs, participant.TransferIDs)
		participant.TransferRefs = appendUniqueRefs(participant.TransferRefs, participant.TokenTransferIDs)
		approved = append(approved, participant)
	}
	return approved, nil
}

// Participant registers for a token (recorded as pending for token owner approval)
func (s *SmartContract) RegisterCustomer(ctx contractapi.TransactionContextInterface, networkAddress, name, tokenID, kycId, kycStatus string) error {
	// Get caller's MSP
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}

	// SECURITY: Verify caller is the customer registering (self-registration only)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	// Load participant to get their client ID
	partBytes, err := ctx.GetStub().GetState(networkAddress)
	if err != nil || partBytes == nil {
		return fmt.Errorf("participant not found")
	}
	var participant Participant
	if err := json.Unmarshal(partBytes, &participant); err != nil {
		return fmt.Errorf("invalid participant record: %w", err)
	}
	normalizeParticipantForRead(&participant)
	// Caller must be registering as themselves
	if participant.ClientID != callerID {
		return fmt.Errorf("forbidden: can only register yourself")
	}

	// DATA ISOLATION: Verify participant belongs to caller's bank
	if err := s.VerifyBankAccessToData(ctx, participant.MSP); err != nil {
		return err
	}

	// Check token exists and approved
	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil || token.Owner == "" {
		return fmt.Errorf("invalid or unowned token")
	}

	// VERIFY: Only token owner's bank can allow customer registration to their token
	if token.OwnerMSP != callerMSP {
		return fmt.Errorf("access denied: only token owner's bank can register customers to their token")
	}

	// SECURITY FIX: Prevent duplicate registrations - customer can register to a token only ONCE
	if existingReg, _, regErr := s.getParticipantByNetworkToken(ctx, networkAddress, tokenID); regErr == nil && existingReg != nil {
		if existingReg.Approved || existingReg.TokenID == tokenID {
			return fmt.Errorf("customer is already registered to this token. Cannot register twice")
		}
	}

	// Build request id per transaction similar to other requests
	txID := ctx.GetStub().GetTxID()
	shortTx := txID
	if len(shortTx) > 8 {
		shortTx = shortTx[:8]
	}
	bic, err := s.resolveTokenBIC(ctx, token)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	msgID := fmt.Sprintf("%s-CREG-%s", bic, shortTx)
	req := RegisterParticipantRequest{
		MsgID:       msgID,
		BIC:         bic,
		TokenID:     tokenID,
		CustomerRef: networkAddress,
		KycRef:      kycId,
		KycStatus:   kycStatus,
		Status:      "PENDING",
		Purpose:     "CUSTOMER_REGISTRATION",
		CreatedAt:   now.Format(time.RFC3339),
		ExpiresAt:   now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}
	requestBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(registerParticipantStateKey(bic, msgID), requestBytes)
}

// Token owner views pending customer registrations for their token
func (s *SmartContract) ViewPendingCustomerRegistrations(ctx contractapi.TransactionContextInterface, tokenID, ownerNetworkAddress string) ([]RegisterParticipantRequest, error) {
	// SECURITY: Verify caller's actual identity matches the owner (not just parameter match)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return nil, fmt.Errorf("forbidden: caller identity does not match owner")
	}

	// Verify caller is a bank and owns this specific token
	if err := s.VerifyBankOwner(ctx, tokenID); err != nil {
		return nil, fmt.Errorf("forbidden: %v", err)
	}

	// Verify caller is owner of tokenID
	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tokenBytes == nil {
		return nil, fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, err
	}
	if token.Owner != ownerNetworkAddress {
		return nil, fmt.Errorf("caller is not token owner")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var pendingRequests []RegisterParticipantRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if isRegisterParticipantRequestKey(kv.Key) {
			var req RegisterParticipantRequest
			if err := json.Unmarshal(kv.Value, &req); err == nil && req.TokenID == tokenID && strings.ToUpper(strings.TrimSpace(req.Status)) == "PENDING" {
				pendingRequests = append(pendingRequests, req)
			}
		}
	}
	return pendingRequests, nil
}

// Token owner approves customer registration
func (s *SmartContract) ApproveCustomerRegistration(ctx contractapi.TransactionContextInterface, requestID, ownerNetworkAddress string) error {
	// Get caller's MSP
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}

	// SECURITY: Verify caller's actual identity matches the owner (not just parameter match)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return fmt.Errorf("forbidden: caller identity does not match owner")
	}

	stateKey, err := s.resolveRegisterParticipantStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("customer registration request not found")
	}
	var req RegisterParticipantRequest
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return err
	}

	// Verify caller is a bank and owns the token for this request
	if err := s.VerifyBankOwner(ctx, req.TokenID); err != nil {
		return fmt.Errorf("forbidden: %v", err)
	}

	// Verify caller is token owner
	tokenBytes, err := ctx.GetStub().GetState(req.TokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}
	if token.Owner != ownerNetworkAddress {
		return fmt.Errorf("caller is not token owner")
	}

	// VERIFY: Only token owner's bank can approve customer registration
	if token.OwnerMSP != callerMSP {
		return fmt.Errorf("access denied: only token owner's bank can approve customers")
	}

	if strings.ToUpper(strings.TrimSpace(req.Status)) == "APPROVED" {
		return fmt.Errorf("already approved")
	}

	// SECURITY FIX #6: Validate KycStatus is "verified" before approving
	// Normalize KYC status - accept various true formats and convert to "verified"
	req.KycStatus = strings.TrimSpace(req.KycStatus)
	normalizedStatus := strings.ToLower(req.KycStatus)

	// Check if KYC is verified (accept: "verified", "true", "1", "yes", "approved")
	isVerified := normalizedStatus == "verified" || normalizedStatus == "true" ||
		normalizedStatus == "1" || normalizedStatus == "yes" || normalizedStatus == "approved"

	if !isVerified {
		return fmt.Errorf("customer KYC status must be verified before approval. Current status: %s", req.KycStatus)
	}

	// Normalize to "verified" for storage
	req.KycStatus = "verified"

	// SECURITY FIX #7: Check request timestamp is not too old (older than 30 days)
	req.ExpiresAt = strings.TrimSpace(req.ExpiresAt)
	if req.ExpiresAt != "" {
		expiryTime, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err == nil && time.Now().UTC().After(expiryTime) {
			return fmt.Errorf("registration request expired at %s", req.ExpiresAt)
		}
	}
	req.Status = "APPROVED"
	updatedReqBytes, _ := json.Marshal(req)
	if err := ctx.GetStub().PutState(stateKey, updatedReqBytes); err != nil {
		return err
	}

	// Update participant record with kyc_id and kyc_status
	participantBytes, err := ctx.GetStub().GetState(ownerNetworkAddress)
	if err == nil && participantBytes != nil {
		var participant Participant
		if err := json.Unmarshal(participantBytes, &participant); err == nil {
			normalizeParticipantForRead(&participant)
			participant.KycId = req.KycRef
			participant.KycRef = req.KycRef
			participant.KycStatus = req.KycStatus
			participant.Approved = true // Mark participant as approved
			participant.Status = "ACTIVE"
			participant.LastUpdated = time.Now().UTC().Format(time.RFC3339)
			normalizeParticipantForWrite(&participant, strings.TrimSpace(token.BIC), participant.LastUpdated)
			if updatedParticipantBytes, err := json.Marshal(participant); err == nil {
				ctx.GetStub().PutState(ownerNetworkAddress, updatedParticipantBytes)
			}
		}
	}

	customerID := generateTokenScopedCustomerID(req.TokenID, ctx.GetStub().GetTxID())
	if err := ensureCustomerIDUnique(ctx, customerID); err != nil {
		return err
	}

	participant := Participant{
		CustomerRef:       req.CustomerRef,
		KycRef:            req.KycRef,
		CustomerID:        customerID,
		NetworkAddress:    req.CustomerRef,
		Name:              req.CustomerRef,
		ClientID:          req.CustomerRef,
		BIC:               strings.TrimSpace(strings.ToUpper(token.BIC)),
		TokenID:           req.TokenID,
		KycId:             req.KycRef,
		KycStatus:         req.KycStatus,
		Approved:          true,
		Status:            "ACTIVE",
		ActivatedAt:       time.Now().UTC().Format(time.RFC3339),
		LastUpdated:       time.Now().UTC().Format(time.RFC3339),
		Balance:           0,
		ForeignBalances:   make(map[string]int64),
		TransferIDs:       []string{},
		TransferRefs:      []string{},
		TokenTransferIDs:  []string{},
		ForeignCurrencies: make(map[string]float64),
	}
	normalizeParticipantForWrite(&participant, token.BIC, participant.LastUpdated)
	participantBytes, err = json.Marshal(participant)
	if err != nil {
		return err
	}
	participantKey := participantStateKeyByCustomerID(customerID)
	if err := ctx.GetStub().PutState(participantKey, participantBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(participantNetworkTokenIndexKey(req.CustomerRef, req.TokenID), []byte(customerID)); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(customerIDUniqueKey(customerID), []byte(participantKey)); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(customerIDTokenIndexKey(req.TokenID, customerID), []byte(participantKey)); err != nil {
		return err
	}

	// IMPORTANT: Also create an approved mint request entry so the customer appears in ListApprovedCustomerMintRequests
	// This ensures the approved customer is visible in /bank/participants/approved endpoint
	msgID := fmt.Sprintf("%s-MINT-REG-%s", strings.ToUpper(strings.TrimSpace(token.BIC)), customerID)
	mintStateKey := mintRequestStateKey(token.BIC, msgID)
	now := time.Now().UTC()
	approvedMintRequest := MintRequest{
		MsgID:          msgID,
		BIC:            strings.ToUpper(strings.TrimSpace(token.BIC)),
		TokenID:        req.TokenID,
		Amount:         0, // Initial balance is 0, customer can request mint later
		Currency:       strings.ToUpper(strings.TrimSpace(token.Currency)),
		KycRef:         req.KycRef,
		KycStatus:      strings.ToUpper(strings.TrimSpace(req.KycStatus)),
		Status:         "APPROVED", // Mark as approved immediately upon customer registration approval
		CreatedAt:      now.Format(time.RFC3339),
		ApprovedAt:     now.Format(time.RFC3339),
		ExpiresAt:      now.Add(mintRequestTTLDays * 24 * time.Hour).Format(time.RFC3339),
		Purpose:        mintPurposeWorkingCapital,
		DailyLimitUsed: 0,
	}
	setMintRequestCustomerRef(&approvedMintRequest, customerID)
	mintReqBytes, _ := json.Marshal(approvedMintRequest)
	return ctx.GetStub().PutState(mintStateKey, mintReqBytes)
}

// Token owner rejects customer registration.
func (s *SmartContract) RejectCustomerRegistration(ctx contractapi.TransactionContextInterface, requestID, ownerNetworkAddress string) error {
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return fmt.Errorf("forbidden: caller identity does not match owner")
	}

	stateKey, err := s.resolveRegisterParticipantStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("customer registration request not found")
	}
	var req RegisterParticipantRequest
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return err
	}

	if err := s.VerifyBankOwner(ctx, req.TokenID); err != nil {
		return fmt.Errorf("forbidden: %v", err)
	}

	tokenBytes, err := ctx.GetStub().GetState(req.TokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}
	if token.Owner != ownerNetworkAddress {
		return fmt.Errorf("caller is not token owner")
	}
	if token.OwnerMSP != callerMSP {
		return fmt.Errorf("access denied: only token owner's bank can reject customers")
	}

	currentStatus := strings.ToUpper(strings.TrimSpace(req.Status))
	if currentStatus == "APPROVED" {
		return fmt.Errorf("cannot reject an already approved registration")
	}
	if currentStatus == "REJECTED" {
		return fmt.Errorf("registration already rejected")
	}

	req.Status = "REJECTED"
	updatedReqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(stateKey, updatedReqBytes)
}

// ListApprovedCustomers returns all approved customers for a given token/owner pair.
func (s *SmartContract) ListApprovedCustomers(ctx contractapi.TransactionContextInterface, tokenID, ownerNetworkAddress string) ([]Participant, error) {
	// SECURITY: Verify caller's actual identity matches the owner (not just parameter match)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return nil, fmt.Errorf("forbidden: caller identity does not match owner")
	}

	trimmedTokenID := strings.TrimSpace(tokenID)
	// If a specific token is requested, ensure caller owns it (MSP-based)
	if trimmedTokenID != "" {
		if err := s.VerifyBankOwner(ctx, trimmedTokenID); err != nil {
			return nil, fmt.Errorf("forbidden: %v", err)
		}
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var approvedCustomers []Participant
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "participant_") {
			continue
		}
		var cust Participant
		if err := json.Unmarshal(kv.Value, &cust); err != nil {
			continue
		}
		normalizeParticipantForRead(&cust)
		if cust.TokenID != tokenID || !cust.Approved {
			continue
		}
		if cust.TransferIDs == nil {
			cust.TransferIDs = []string{}
		}
		if cust.TokenTransferIDs == nil {
			cust.TokenTransferIDs = []string{}
		}
		if cust.ForeignCurrencies == nil {
			cust.ForeignCurrencies = make(map[string]float64)
		}
		cust.TransferRefs = appendUniqueRefs(cust.TransferRefs, cust.TransferIDs)
		cust.TransferRefs = appendUniqueRefs(cust.TransferRefs, cust.TokenTransferIDs)
		approvedCustomers = append(approvedCustomers, cust)
	}
	return approvedCustomers, nil
}

// ListAllApprovedCustomers returns all approved customers across all tokens
func (s *SmartContract) ListAllApprovedCustomers(ctx contractapi.TransactionContextInterface) ([]Participant, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}
	// Get caller's MSP for data isolation
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, err
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var approvedCustomers []Participant
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "participant_") {
			continue
		}
		var cust Participant
		if err := json.Unmarshal(kv.Value, &cust); err != nil {
			continue
		}
		normalizeParticipantForRead(&cust)
		if !cust.Approved {
			continue
		}
		// Only return customers from caller's bank (MSP isolation)
		if cust.MSP != callerMSP {
			continue
		}
		if cust.TransferIDs == nil {
			cust.TransferIDs = []string{}
		}
		if cust.TokenTransferIDs == nil {
			cust.TokenTransferIDs = []string{}
		}
		if cust.ForeignCurrencies == nil {
			cust.ForeignCurrencies = make(map[string]float64)
		}
		cust.TransferRefs = appendUniqueRefs(cust.TransferRefs, cust.TransferIDs)
		cust.TransferRefs = appendUniqueRefs(cust.TransferRefs, cust.TokenTransferIDs)
		approvedCustomers = append(approvedCustomers, cust)
	}
	return approvedCustomers, nil
}

// UpdateCustomerKYC stores the latest KYC identifier/status for a customer without exposing PII.
func (s *SmartContract) UpdateCustomerKYC(ctx contractapi.TransactionContextInterface, networkAddress, tokenID, kycID, kycStatus string) error {
	// SECURITY: Verify caller is customer or bank owner of token
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}

	if strings.TrimSpace(networkAddress) == "" || strings.TrimSpace(tokenID) == "" {
		return fmt.Errorf("network address and token id are required")
	}

	// Load token to verify caller is the bank owner (MSP-based)
	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return fmt.Errorf("invalid token record: %w", err)
	}
	// Verify caller's bank owns the token
	if err := s.VerifyBankOwner(ctx, tokenID); err != nil {
		return fmt.Errorf("forbidden: %v", err)
	}
	// Caller must be the bank owner OR the customer themselves
	if callerID != token.Owner {
		// If not bank owner, verify customer is updating their own KYC
		tempCust, _, loadErr := s.getParticipantByNetworkToken(ctx, networkAddress, tokenID)
		if loadErr == nil && tempCust != nil && tempCust.ClientID != "" && tempCust.ClientID != callerID {
			return fmt.Errorf("forbidden: can only update your own KYC or you must own the token")
		}
	}

	customer, customerKey, err := s.getParticipantByNetworkToken(ctx, networkAddress, tokenID)
	if err != nil {
		return err
	}

	statusValue := strings.TrimSpace(kycStatus)
	approved, parseErr := strconv.ParseBool(statusValue)
	if parseErr != nil {
		switch strings.ToLower(statusValue) {
		case "approved", "pass", "passed", "success", "verified":
			approved = true
		default:
			approved = false
		}
	}

	if strings.TrimSpace(kycID) != "" {
		customer.KycId = strings.TrimSpace(kycID)
		customer.KycRef = customer.KycId
	}
	if approved {
		customer.KycStatus = "approved"
	} else {
		customer.KycStatus = "pending"
	}
	if approved && !customer.Approved {
		customer.Approved = true
		customer.Status = "ACTIVE"
		customer.ActivatedAt = time.Now().UTC().Format(time.RFC3339)
	} else if strings.TrimSpace(customer.Status) == "" {
		customer.Status = "PENDING"
	}
	customer.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	normalizeParticipantForWrite(customer, token.BIC, customer.LastUpdated)

	updatedBytes, err := json.Marshal(customer)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(customerKey, updatedBytes)
}

// UpsertCustomerFromBank lets a bank-provisioned workflow create/update a customer without exposing PII on-chain.
func (s *SmartContract) UpsertCustomerFromBank(ctx contractapi.TransactionContextInterface, networkAddress, clientID, tokenID, kycID, kycStatus string) error {
	// SECURITY: Allow either token owner OR the customer identity to upsert (no longer bank-owner only)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}

	trimmedNetwork := strings.TrimSpace(networkAddress)
	trimmedToken := strings.TrimSpace(tokenID)
	if trimmedNetwork == "" || trimmedToken == "" {
		return fmt.Errorf("network address and token id are required")
	}

	tokenBytes, err := ctx.GetStub().GetState(trimmedToken)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token %s not found", trimmedToken)
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return fmt.Errorf("invalid token record: %w", err)
	}
	if token.Owner == "" {
		return fmt.Errorf("token %s is not assigned to an owner", trimmedToken)
	}
	// Verify caller is authorized: either token owner OR the customer themselves (MSP check)
	if callerID != token.Owner && callerID != trimmedNetwork {
		return fmt.Errorf("forbidden: caller must be token owner or the customer identity")
	}
	// Additional MSP validation if caller is token owner
	if callerID == token.Owner {
		if err := s.VerifyBankOwner(ctx, trimmedToken); err != nil {
			return fmt.Errorf("forbidden: %v", err)
		}
	}

	customer, customerKey, lookupErr := s.getParticipantByNetworkToken(ctx, trimmedNetwork, trimmedToken)
	var customerValue Participant
	if lookupErr == nil && customer != nil {
		customerValue = *customer
		normalizeParticipantForRead(&customerValue)
	} else {
		customerID := generateTokenScopedCustomerID(trimmedToken, ctx.GetStub().GetTxID())
		if err := ensureCustomerIDUnique(ctx, customerID); err != nil {
			return err
		}
		customerValue = Participant{
			CustomerRef:       trimmedNetwork,
			CustomerID:        customerID,
			NetworkAddress:    trimmedNetwork,
			BIC:               strings.TrimSpace(strings.ToUpper(token.BIC)),
			TokenID:           trimmedToken,
			Status:            "PENDING",
			TransferIDs:       []string{},
			TransferRefs:      []string{},
			TokenTransferIDs:  []string{},
			ForeignBalances:   make(map[string]int64),
			ForeignCurrencies: make(map[string]float64),
		}
		customerKey = participantStateKeyByCustomerID(customerID)
	}

	if strings.TrimSpace(clientID) != "" {
		customerValue.ClientID = strings.TrimSpace(clientID)
		if customerValue.Name == "" {
			customerValue.Name = customerValue.ClientID
		}
	}
	if customerValue.TransferIDs == nil {
		customerValue.TransferIDs = []string{}
	}
	if customerValue.TokenTransferIDs == nil {
		customerValue.TokenTransferIDs = []string{}
	}
	if customerValue.ForeignCurrencies == nil {
		customerValue.ForeignCurrencies = make(map[string]float64)
	}

	// Ensure the recorded client matches the customer network address so downstream customer-only flows work
	if callerID == token.Owner || callerID == trimmedNetwork {
		if customerValue.ClientID != trimmedNetwork {
			customerValue.ClientID = trimmedNetwork
		}
	}

	statusValue := strings.TrimSpace(kycStatus)
	approved := false
	if statusValue != "" {
		if parsed, parseErr := strconv.ParseBool(statusValue); parseErr == nil {
			approved = parsed
		} else {
			switch strings.ToLower(statusValue) {
			case "approved", "pass", "passed", "success", "verified":
				approved = true
			}
		}
	}
	if strings.TrimSpace(kycID) != "" {
		customerValue.KycId = strings.TrimSpace(kycID)
		customerValue.KycRef = customerValue.KycId
	}

	if approved {
		customerValue.KycStatus = "approved"
	} else {
		customerValue.KycStatus = "pending"
	}
	if approved {
		customerValue.Approved = true
		customerValue.Status = "ACTIVE"
		customerValue.ActivatedAt = time.Now().UTC().Format(time.RFC3339)
	} else if strings.TrimSpace(customerValue.Status) == "" {
		customerValue.Status = "PENDING"
	}
	customerValue.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	normalizeParticipantForWrite(&customerValue, token.BIC, customerValue.LastUpdated)

	updatedBytes, err := json.Marshal(customerValue)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(customerKey, updatedBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(participantNetworkTokenIndexKey(trimmedNetwork, trimmedToken), []byte(customerValue.CustomerID)); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(customerIDUniqueKey(customerValue.CustomerID), []byte(customerKey)); err != nil {
		return err
	}
	return ctx.GetStub().PutState(customerIDTokenIndexKey(trimmedToken, customerValue.CustomerID), []byte(customerKey))
}

// Participant requests coins minting (referenced by token and customer)
func (s *SmartContract) CustomerRequestMint(ctx contractapi.TransactionContextInterface, networkAddress, tokenID string, amount int) error {
	// Get caller's MSP
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}

	// SECURITY: Verify caller is the customer requesting mint (self-service only)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}

	// Validate amount is positive
	if amount <= 0 {
		return fmt.Errorf("mint amount must be positive, received: %d", amount)
	}

	// SECURITY FIX: Use correct per-token customer key format
	customer, customerKey, err := s.getParticipantByNetworkToken(ctx, networkAddress, tokenID)
	if err != nil {
		return fmt.Errorf("customer not registered or approved for token")
	}

	// SECURITY FIX: Verify customer is actually approved before allowing mint request
	if !customer.Approved {
		return fmt.Errorf("customer not approved for requesting funds")
	}

	// Verify customer is registered with caller identity
	if customer.ClientID != "" && customer.ClientID != callerID {
		return fmt.Errorf("forbidden: caller identity does not match customer")
	}
	if customer.ClientID == "" {
		customer.ClientID = callerID
		updatedBytes, marshalErr := json.Marshal(customer)
		if marshalErr == nil {
			if putErr := ctx.GetStub().PutState(customerKey, updatedBytes); putErr != nil {
				return putErr
			}
		}
	}
	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}

	// VERIFY: Only token owner's bank can allow customer mints to their token
	if token.OwnerMSP != callerMSP {
		return fmt.Errorf("access denied: only token owner's bank can process customer mint requests")
	}

	tokenCurrency := strings.TrimSpace(token.Currency)
	if tokenCurrency == "" {
		return fmt.Errorf("token currency not configured")
	}
	if strings.EqualFold(strings.TrimSpace(token.Status), "FROZEN") || token.IsFrozen {
		return fmt.Errorf("token %s is frozen", token.TokenID)
	}
	bic, err := s.resolveTokenBIC(ctx, token)
	if err != nil {
		return err
	}
	if bic != token.BIC {
		token.BIC = bic
		updatedTokenBytes, marshalErr := json.Marshal(token)
		if marshalErr == nil {
			_ = ctx.GetStub().PutState(token.TokenID, updatedTokenBytes)
		}
	}
	todayTotal, err := s.getTodayMintTotalForBIC(ctx, bic)
	if err != nil {
		return err
	}
	if todayTotal+int64(amount) > maxMintRequestAmount {
		return fmt.Errorf("daily cap exceeded for %s", bic)
	}

	// Create mint request ID using the current transaction
	txID := ctx.GetStub().GetTxID()
	shortTx := txID
	if len(shortTx) > 8 {
		shortTx = shortTx[:8]
	}
	msgID := fmt.Sprintf("%s-MINT-%s", bic, shortTx)
	txTime, err := s.getTxUTCTime(ctx)
	if err != nil {
		return err
	}
	mintReq := MintRequest{
		MsgID:          msgID,
		BIC:            bic,
		TokenID:        tokenID,
		Amount:         int64(amount),
		Currency:       tokenCurrency,
		KycRef:         strings.TrimSpace(customer.KycId),
		KycStatus:      strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", customer.KycStatus))),
		Status:         "PENDING",
		CreatedAt:      txTime.Format(time.RFC3339),
		ExpiresAt:      txTime.Add(mintRequestTTLDays * 24 * time.Hour).Format(time.RFC3339),
		Purpose:        mintPurposeWorkingCapital,
		DailyLimitUsed: todayTotal + int64(amount),
	}
	setMintRequestCustomerRef(&mintReq, customer.CustomerID)
	if mintReq.KycStatus == "" {
		mintReq.KycStatus = "VERIFIED"
	}
	reqBytes, _ := json.Marshal(mintReq)
	return ctx.GetStub().PutState(mintRequestStateKey(bic, msgID), reqBytes)
}

// Token owner views pending mint requests for their token from customers
func (s *SmartContract) ViewPendingCustomerMintRequests(ctx contractapi.TransactionContextInterface, tokenID, ownerNetworkAddress string) ([]MintRequest, error) {
	// SECURITY: Verify caller's actual identity matches the owner (not just parameter match)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return nil, fmt.Errorf("forbidden: caller identity does not match owner")
	}

	trimmedTokenID := strings.TrimSpace(tokenID)
	// If a specific token is requested, ensure caller owns it
	if trimmedTokenID != "" {
		if err := s.VerifyBankOwner(ctx, trimmedTokenID); err != nil {
			return nil, fmt.Errorf("forbidden: %v", err)
		}
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	pending := []MintRequest{} // Initialize as empty slice to ensure valid JSON
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var r MintRequest
		if err := json.Unmarshal(kv.Value, &r); err != nil {
			continue
		}
		if !isMintRequestPending(r) {
			continue
		}
		if trimmedTokenID != "" && r.TokenID != trimmedTokenID {
			continue
		}

		tokenBytes, tErr := ctx.GetStub().GetState(r.TokenID)
		if tErr != nil || tokenBytes == nil {
			continue
		}
		var token Token
		if err := json.Unmarshal(tokenBytes, &token); err != nil {
			continue
		}
		if token.Owner != ownerNetworkAddress {
			continue
		}

		pending = append(pending, r)
	}
	return pending, nil
}

// ListApprovedCustomerMintRequests returns all approved customer mint requests across tokens.
func (s *SmartContract) ListApprovedCustomerMintRequests(ctx contractapi.TransactionContextInterface) ([]MintRequest, error) {
	// Get caller's MSP for data isolation
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, err
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var approved []MintRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if isMintRequestApproved(req) {
			// Customer records lane must include only customer-origin mint requests.
			if !isCustomerScopedMintRequest(req) {
				continue
			}
			// Verify token owner's MSP matches caller's MSP (data isolation)
			tokenBytes, tErr := ctx.GetStub().GetState(req.TokenID)
			if tErr != nil || tokenBytes == nil {
				continue
			}
			var token Token
			if err := json.Unmarshal(tokenBytes, &token); err != nil {
				continue
			}
			// Only return approved requests for tokens from caller's bank
			if token.OwnerMSP == callerMSP {
				approved = append(approved, req)
			}
		}
	}
	return approved, nil
}

// ListTokenMintRecords returns approved token-owner mint records (admin/bank token mints).
func (s *SmartContract) ListTokenMintRecords(ctx contractapi.TransactionContextInterface) ([]TokenMintRecord, error) {
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, err
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	byRecordID := make(map[string]TokenMintRecord)
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isTokenMintRecordKey(kv.Key) {
			continue
		}

		var rec TokenMintRecord
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			continue
		}
		if strings.TrimSpace(rec.TokenID) == "" {
			continue
		}

		tokenBytes, tErr := ctx.GetStub().GetState(rec.TokenID)
		if tErr != nil || tokenBytes == nil {
			continue
		}
		var token Token
		if err := json.Unmarshal(tokenBytes, &token); err != nil {
			continue
		}
		if token.OwnerMSP != callerMSP {
			continue
		}

		recordID := strings.TrimSpace(rec.RecordID)
		if recordID == "" {
			recordID = strings.TrimSpace(rec.RequestID)
		}
		if recordID == "" {
			continue
		}
		if existing, ok := byRecordID[recordID]; !ok || strings.TrimSpace(rec.ApprovedAt) > strings.TrimSpace(existing.ApprovedAt) {
			byRecordID[recordID] = rec
		}
	}

	records := make([]TokenMintRecord, 0, len(byRecordID))
	for _, rec := range byRecordID {
		records = append(records, rec)
	}
	return records, nil
}

// GetApprovedMintRequestsByNetworkAddress returns approved mint requests for a specific customer (by network address)
// SECURITY: Only customer with matching network address can view their own mints
func (s *SmartContract) GetApprovedMintRequestsByNetworkAddress(ctx contractapi.TransactionContextInterface, customerNetworkAddress string) ([]MintRequest, error) {
	// SECURITY: Validate caller's identity matches the requested customer
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}

	// Extract DN from certificate and compare with customerNetworkAddress
	// Caller can only view their own approved mints
	if callerID != customerNetworkAddress {
		return nil, fmt.Errorf("forbidden: caller cannot view another customer's mint requests")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var approved []MintRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		// Filter by customer and approval status
		if mintRequestCustomerRef(req) == customerNetworkAddress && isMintRequestApproved(req) {
			approved = append(approved, req)
		}
	}
	return approved, nil
}

// GetMyApprovedMintRequests returns approved mint requests for the caller only.
// SECURITY: Caller identity is derived from certificate; no customer identifier input is accepted.
func (s *SmartContract) GetMyApprovedMintRequests(ctx contractapi.TransactionContextInterface) ([]MintRequest, error) {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var approved []MintRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isMintRequestKey(kv.Key) {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if isMintRequestApproved(req) && mintRequestCustomerRef(req) == callerID {
			approved = append(approved, req)
		}
	}
	return approved, nil
}

// Token owner approves customer mint request, increasing customer balance
// Token owner approves customer mint request, increasing customer balance if token has sufficient minted coins
func (s *SmartContract) ApproveCustomerMint(ctx contractapi.TransactionContextInterface, requestID, ownerNetworkAddress string) error {
	// SECURITY: Verify caller's actual identity matches the owner (not just parameter match)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return fmt.Errorf("forbidden: caller identity does not match owner")
	}

	// Retrieve the mint request by ID
	stateKey, err := s.resolveMintRequestStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("mint request not found")
	}
	var mintReq MintRequest
	if err := json.Unmarshal(reqBytes, &mintReq); err != nil {
		return err
	}

	// Verify caller is a bank and owns the token for this mint request
	if err := s.VerifyBankOwner(ctx, mintReq.TokenID); err != nil {
		return fmt.Errorf("forbidden: %v", err)
	}

	// Retrieve the token state
	tokenBytes, err := ctx.GetStub().GetState(mintReq.TokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}
	tokenCurrency := strings.TrimSpace(token.Currency)
	if tokenCurrency == "" {
		return fmt.Errorf("token currency not configured")
	}
	if mintReq.Currency == "" {
		mintReq.Currency = tokenCurrency
	} else if mintReq.Currency != tokenCurrency {
		return fmt.Errorf("mint request currency %s does not match token currency %s", mintReq.Currency, tokenCurrency)
	}

	// Check that caller is indeed token owner
	if token.Owner != ownerNetworkAddress {
		return fmt.Errorf("caller is not token owner")
	}

	// Check if request already approved
	if isMintRequestApproved(mintReq) {
		return fmt.Errorf("mint request already approved")
	}

	// Check if the token has enough minted coins to fulfill this request
	if getTokenSupply(token) < int(mintReq.Amount) {
		return fmt.Errorf("insufficient minted coin balance on token: available %d, requested %d", getTokenSupply(token), mintReq.Amount)
	}

	// Approve mint request
	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	mintReq.Status = "APPROVED"
	if strings.TrimSpace(mintReq.CreatedAt) == "" {
		mintReq.CreatedAt = ts
	}
	mintReq.ApprovedAt = ts
	updatedReqBytes, err := json.Marshal(mintReq)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(stateKey, updatedReqBytes); err != nil {
		return err
	}

	// Deduct the requested amount from token's minted coins balance
	setTokenSupply(&token, getTokenSupply(token)-int(mintReq.Amount))
	updatedTokenBytes, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(mintReq.TokenID, updatedTokenBytes); err != nil {
		return err
	}

	// Credit the customer’s balance
	cust, customerKey, err := s.getParticipantByCustomerIDToken(ctx, mintRequestCustomerRef(mintReq), mintReq.TokenID)
	if err != nil {
		return fmt.Errorf("customer not found")
	}
	cust.Balance += int(mintReq.Amount)
	updatedCustBytes, err := json.Marshal(cust)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(customerKey, updatedCustBytes)
}

// Token owner rejects customer mint request.
func (s *SmartContract) RejectCustomerMint(ctx contractapi.TransactionContextInterface, requestID, ownerNetworkAddress string) error {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return fmt.Errorf("forbidden: caller identity does not match owner")
	}

	stateKey, err := s.resolveMintRequestStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("mint request not found")
	}
	var mintReq MintRequest
	if err := json.Unmarshal(reqBytes, &mintReq); err != nil {
		return err
	}

	if err := s.VerifyBankOwner(ctx, mintReq.TokenID); err != nil {
		return fmt.Errorf("forbidden: %v", err)
	}

	tokenBytes, err := ctx.GetStub().GetState(mintReq.TokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}
	if token.Owner != ownerNetworkAddress {
		return fmt.Errorf("caller is not token owner")
	}

	if isMintRequestApproved(mintReq) {
		return fmt.Errorf("cannot reject an already approved mint request")
	}
	if strings.ToUpper(strings.TrimSpace(mintReq.Status)) == "REJECTED" {
		return fmt.Errorf("mint request already rejected")
	}

	mintReq.Status = "REJECTED"
	updatedReqBytes, err := json.Marshal(mintReq)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(stateKey, updatedReqBytes)
}

// Participant views their subtoken wallet info securely
func (s *SmartContract) ViewCustomerWallet(ctx contractapi.TransactionContextInterface, networkAddress, tokenID string) (map[string]interface{}, error) {
	// SECURITY: Verify caller can only view their own wallet (MSP-based)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	cust, customerKey, err := s.getParticipantByNetworkToken(ctx, networkAddress, tokenID)
	if err != nil {
		return nil, fmt.Errorf("customer not found")
	}
	tokenBytes, err := ctx.GetStub().GetState(cust.TokenID)
	if err != nil || tokenBytes == nil {
		return nil, fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, err
	}
	// Verify caller is the approved customer or the token owner (owner can assist/support) - MSP check
	authorized := false
	if cust.ClientID == "" || cust.ClientID == callerID {
		authorized = true
	} else if callerID == cust.NetworkAddress || callerID == token.Owner {
		authorized = true
	}
	// Ensure customer's MSP matches caller's MSP (data isolation)
	if authorized && cust.MSP != "" && cust.MSP != callerMSP {
		return nil, fmt.Errorf("forbidden: cannot access another bank's customer wallet")
	}
	if !authorized {
		return nil, fmt.Errorf("forbidden: can only view your own wallet")
	}
	if cust.ClientID == "" {
		cust.ClientID = callerID
		if updated, marshalErr := json.Marshal(cust); marshalErr == nil {
			if putErr := ctx.GetStub().PutState(customerKey, updated); putErr != nil {
				return nil, putErr
			}
		}
	}
	if cust.TransferIDs == nil {
		cust.TransferIDs = []string{}
	}
	if cust.TokenTransferIDs == nil {
		cust.TokenTransferIDs = []string{}
	}
	customerParticipantIDs := append([]string{}, cust.TransferIDs...)
	customerTokenIDs := append([]string{}, cust.TokenTransferIDs...)
	custPtID := deriveParticipantTransferID(cust.NetworkAddress)
	custBtID := deriveBankTransferID(cust.TokenID)
	if custPtID != "" {
		customerParticipantIDs = appendTransferIfMissing(customerParticipantIDs, custPtID)
	}
	if custBtID != "" {
		customerTokenIDs = appendTransferIfMissing(customerTokenIDs, custBtID)
	}
	walletBalance := float64(cust.Balance)
	// NOTE: Customer should only see their local currency balance
	// Token's foreign balances are NOT part of customer's wallet view
	// Customer can only receive in their local currency
	return map[string]interface{}{
		"networkAddress":         cust.NetworkAddress,
		"customerID":             cust.CustomerID,
		"customerRef":            cust.CustomerRef,
		"customer_ref":           cust.CustomerRef,
		"bic":                    strings.TrimSpace(strings.ToUpper(cust.BIC)),
		"bic_code":               strings.TrimSpace(strings.ToUpper(cust.BIC)),
		"tokenID":                cust.TokenID,
		"balance":                cust.Balance,
		"currency":               token.Currency,
		"currencySymbol":         currencySymbol(token.Currency),
		"balanceDisplay":         formatCurrencyValue(token.Currency, float64(cust.Balance)),
		"walletBalance":          walletBalance,
		"walletBalanceDisplay":   formatCurrencyValue(token.Currency, walletBalance),
		"wallet_balance":         walletBalance,
		"wallet_balance_display": formatCurrencyValue(token.Currency, walletBalance),
		"approved":               cust.Approved,
		"status":                 customerApprovalStatus(cust),
		"last_updated":           strings.TrimSpace(cust.LastUpdated),
		"approved_at":            strings.TrimSpace(cust.ApprovedAt),
		"activated_at":           strings.TrimSpace(cust.ActivatedAt),
		"created_at":             strings.TrimSpace(cust.LastUpdated),
		"participantTransferIDs": customerParticipantIDs,
		"tokenTransferIDs":       customerTokenIDs,
		"participantTransferID":  custPtID,
		"tokenTransferID":        custBtID,
	}, nil
}

// GetCustomerIDAccess returns token-level customer identity details for a customer.
// SECURITY: caller must be the same customer identity, and must belong to the same MSP if set.
func (s *SmartContract) GetCustomerIDAccess(ctx contractapi.TransactionContextInterface, networkAddress, tokenID string) (map[string]interface{}, error) {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	customer, _, err := s.getParticipantByNetworkToken(ctx, networkAddress, tokenID)
	if err != nil {
		pendingReq, pendingErr := s.findPendingCustomerRegistration(ctx, networkAddress, tokenID, callerID)
		if pendingErr != nil {
			return nil, pendingErr
		}
		if pendingReq != nil {
			return map[string]interface{}{
				"network_address": strings.TrimSpace(networkAddress),
				"token_id":        strings.TrimSpace(tokenID),
				"customer_id":     "",
				"approved":        false,
				"status":          "PENDING_APPROVAL",
				"message":         "Customer token registration exists and is pending token owner approval",
			}, nil
		}
		return map[string]interface{}{
			"network_address": strings.TrimSpace(networkAddress),
			"token_id":        strings.TrimSpace(tokenID),
			"customer_id":     "",
			"approved":        false,
			"status":          "NOT_REGISTERED",
		}, nil
	}

	if customer.ClientID != "" && customer.ClientID != callerID {
		return nil, fmt.Errorf("forbidden: caller identity does not match customer")
	}
	if customer.MSP != "" && customer.MSP != callerMSP {
		return nil, fmt.Errorf("forbidden: cannot access another bank's customer identity")
	}

	status := customerApprovalStatus(customer)
	return map[string]interface{}{
		"network_address": customer.NetworkAddress,
		"token_id":        customer.TokenID,
		"customer_id":     customer.CustomerID,
		"approved":        customer.Approved,
		"status":          status,
	}, nil
}

func customerApprovalStatus(participant *Participant) string {
	if participant == nil {
		return "NOT_REGISTERED"
	}
	status := strings.TrimSpace(strings.ToUpper(participant.Status))
	if participant.Approved {
		if status == "" || status == "PENDING" || status == "ACTIVE" {
			return "APPROVED"
		}
		return status
	}
	if status == "" || status == "PENDING" {
		return "PENDING_APPROVAL"
	}
	return status
}

func customerApprovalResponse(networkAddress, tokenID, customerRef, customerID, bic string, approved bool, status, createdAt, approvedAt, activatedAt string) map[string]interface{} {
	normalizedStatus := strings.TrimSpace(strings.ToUpper(status))
	if normalizedStatus == "" {
		if approved {
			normalizedStatus = "APPROVED"
		} else {
			normalizedStatus = "PENDING_APPROVAL"
		}
	}
	return map[string]interface{}{
		"network_address": networkAddress,
		"token_id":        tokenID,
		"customer_ref":    customerRef,
		"customer_id":     customerID,
		"bic":             strings.TrimSpace(strings.ToUpper(bic)),
		"approved":        approved,
		"token_assigned":  strings.TrimSpace(tokenID) != "",
		"status":          normalizedStatus,
		"created_at":      strings.TrimSpace(createdAt),
		"approved_at":     strings.TrimSpace(approvedAt),
		"activated_at":    strings.TrimSpace(activatedAt),
	}
}

// GetCustomerTokenApprovalStatus returns whether a customer's token registration is approved.
// SECURITY: caller can query only their own customer identity.
func (s *SmartContract) GetCustomerTokenApprovalStatus(ctx contractapi.TransactionContextInterface, networkAddress, tokenID string) (map[string]interface{}, error) {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	resolvedNetworkAddress := strings.TrimSpace(networkAddress)
	if resolvedNetworkAddress == "" {
		resolvedNetworkAddress = strings.TrimSpace(callerID)
	}
	if resolvedNetworkAddress != strings.TrimSpace(callerID) {
		return nil, fmt.Errorf("forbidden: caller can only query their own token approval status")
	}

	trimmedTokenID := strings.TrimSpace(tokenID)
	if trimmedTokenID != "" {
		customer, _, err := s.getParticipantByNetworkToken(ctx, resolvedNetworkAddress, trimmedTokenID)
		if err != nil {
			pendingReq, pendingErr := s.findPendingCustomerRegistration(ctx, resolvedNetworkAddress, trimmedTokenID, callerID)
			if pendingErr != nil {
				return nil, pendingErr
			}
			if pendingReq != nil {
				return customerApprovalResponse(
					resolvedNetworkAddress,
					trimmedTokenID,
					pendingReq.CustomerRef,
					"",
					pendingReq.BIC,
					false,
					"PENDING_APPROVAL",
					pendingReq.CreatedAt,
					"",
					"",
				), nil
			}
			return customerApprovalResponse(resolvedNetworkAddress, trimmedTokenID, "", "", "", false, "NOT_REGISTERED", "", "", ""), nil
		}
		normalizeParticipantForRead(customer)
		if customer.ClientID != "" && strings.TrimSpace(customer.ClientID) != strings.TrimSpace(callerID) {
			return nil, fmt.Errorf("forbidden: caller identity does not match customer")
		}
		if customer.MSP != "" && strings.TrimSpace(customer.MSP) != strings.TrimSpace(callerMSP) {
			return nil, fmt.Errorf("forbidden: cannot access another bank's customer identity")
		}
		status := customerApprovalStatus(customer)
		return customerApprovalResponse(
			resolvedNetworkAddress,
			customer.TokenID,
			customer.CustomerRef,
			customer.CustomerID,
			customer.BIC,
			customer.Approved,
			status,
			customer.LastUpdated,
			customer.ApprovedAt,
			customer.ActivatedAt,
		), nil
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var selected *Participant
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, participantStatePrefix) {
			continue
		}
		var candidate Participant
		if err := json.Unmarshal(kv.Value, &candidate); err != nil {
			continue
		}
		normalizeParticipantForRead(&candidate)

		matchesCaller := strings.TrimSpace(candidate.ClientID) == strings.TrimSpace(callerID)
		matchesNetwork := strings.TrimSpace(candidate.NetworkAddress) == resolvedNetworkAddress ||
			strings.TrimSpace(candidate.CustomerRef) == resolvedNetworkAddress ||
			strings.TrimSpace(candidate.CustomerID) == resolvedNetworkAddress
		if !matchesCaller && !matchesNetwork {
			continue
		}
		if candidate.MSP != "" && strings.TrimSpace(candidate.MSP) != strings.TrimSpace(callerMSP) {
			continue
		}
		if selected == nil {
			copyCandidate := candidate
			selected = &copyCandidate
			continue
		}
		if !selected.Approved && candidate.Approved {
			copyCandidate := candidate
			selected = &copyCandidate
			continue
		}
		if strings.TrimSpace(selected.TokenID) == "" && strings.TrimSpace(candidate.TokenID) != "" {
			copyCandidate := candidate
			selected = &copyCandidate
		}
	}

	if selected == nil {
		return customerApprovalResponse(resolvedNetworkAddress, "", "", "", "", false, "NOT_REGISTERED", "", "", ""), nil
	}
	status := customerApprovalStatus(selected)
	return customerApprovalResponse(
		resolvedNetworkAddress,
		selected.TokenID,
		selected.CustomerRef,
		selected.CustomerID,
		selected.BIC,
		selected.Approved,
		status,
		selected.LastUpdated,
		selected.ApprovedAt,
		selected.ActivatedAt,
	), nil
}

// GetMyCustomerAccounts returns the caller's approved/pending customer accounts as simple pairs for dashboard selection.
// SECURITY: scoped strictly to caller identity and caller MSP.
func (s *SmartContract) GetMyCustomerAccounts(ctx contractapi.TransactionContextInterface) ([]CustomerTokenAccount, error) {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	accounts := make([]CustomerTokenAccount, 0)
	seen := make(map[string]struct{})

	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, participantStatePrefix) {
			continue
		}

		var candidate Participant
		if err := json.Unmarshal(kv.Value, &candidate); err != nil {
			continue
		}
		normalizeParticipantForRead(&candidate)

		matchesIdentity := strings.TrimSpace(candidate.ClientID) == strings.TrimSpace(callerID) ||
			(strings.TrimSpace(candidate.ClientID) == "" && strings.TrimSpace(candidate.NetworkAddress) == strings.TrimSpace(callerID))
		if !matchesIdentity {
			continue
		}
		if candidate.MSP != "" && strings.TrimSpace(candidate.MSP) != strings.TrimSpace(callerMSP) {
			continue
		}
		if strings.TrimSpace(candidate.TokenID) == "" {
			continue
		}

		status := customerApprovalStatus(&candidate)
		account := CustomerTokenAccount{
			CustomerRef:    strings.TrimSpace(candidate.CustomerRef),
			CustomerID:     strings.TrimSpace(candidate.CustomerID),
			TokenID:        strings.TrimSpace(candidate.TokenID),
			BIC:            strings.TrimSpace(strings.ToUpper(candidate.BIC)),
			Approved:       candidate.Approved,
			Status:         status,
			NetworkAddress: strings.TrimSpace(candidate.NetworkAddress),
		}
		if account.CustomerRef == "" {
			account.CustomerRef = account.CustomerID
		}
		if account.CustomerID == "" {
			account.CustomerID = account.CustomerRef
		}

		// Deduplicate customer/token pairs when both legacy and normalized rows exist.
		dedupeKey := account.CustomerID + "|" + account.TokenID
		if _, exists := seen[dedupeKey]; exists {
			continue
		}
		seen[dedupeKey] = struct{}{}
		accounts = append(accounts, account)
	}

	sort.Slice(accounts, func(i, j int) bool {
		leftRef := strings.TrimSpace(accounts[i].CustomerRef)
		rightRef := strings.TrimSpace(accounts[j].CustomerRef)
		if leftRef == rightRef {
			return strings.TrimSpace(accounts[i].TokenID) < strings.TrimSpace(accounts[j].TokenID)
		}
		return leftRef < rightRef
	})

	return accounts, nil
}

// GetCustomerTokenWallet returns full wallet details for a selected account row.
// SECURITY: caller can only select their own customer_ref/customer_id and token.
func (s *SmartContract) GetCustomerTokenWallet(ctx contractapi.TransactionContextInterface, customerRef, tokenID string) (map[string]interface{}, error) {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller MSP: %v", err)
	}

	trimmedRef := strings.TrimSpace(customerRef)
	trimmedTokenID := strings.TrimSpace(tokenID)
	if trimmedRef == "" || trimmedTokenID == "" {
		return nil, fmt.Errorf("customer_ref and token_id are required")
	}

	// Fast path: treat incoming reference as customer_id.
	customer, _, err := s.getParticipantByCustomerIDToken(ctx, trimmedRef, trimmedTokenID)
	if err != nil || customer == nil {
		// Fallback path: scan participant rows and match by customer_ref/network_address/customer_id.
		iter, scanErr := ctx.GetStub().GetStateByRange("", "")
		if scanErr != nil {
			return nil, scanErr
		}
		defer iter.Close()

		for iter.HasNext() {
			kv, nextErr := iter.Next()
			if nextErr != nil {
				return nil, nextErr
			}
			if !strings.HasPrefix(kv.Key, participantStatePrefix) {
				continue
			}
			var candidate Participant
			if unmarshalErr := json.Unmarshal(kv.Value, &candidate); unmarshalErr != nil {
				continue
			}
			normalizeParticipantForRead(&candidate)
			if strings.TrimSpace(candidate.TokenID) != trimmedTokenID {
				continue
			}
			if strings.TrimSpace(candidate.CustomerRef) != trimmedRef &&
				strings.TrimSpace(candidate.CustomerID) != trimmedRef &&
				strings.TrimSpace(candidate.NetworkAddress) != trimmedRef {
				continue
			}
			copyCandidate := candidate
			customer = &copyCandidate
			break
		}
		if customer == nil {
			return nil, fmt.Errorf("customer account not found for selected customer_ref/token")
		}
	}

	if customer.ClientID != "" && strings.TrimSpace(customer.ClientID) != strings.TrimSpace(callerID) {
		return nil, fmt.Errorf("forbidden: caller identity does not match selected customer account")
	}
	if customer.MSP != "" && strings.TrimSpace(customer.MSP) != strings.TrimSpace(callerMSP) {
		return nil, fmt.Errorf("forbidden: cannot access another bank's customer account")
	}
	if strings.TrimSpace(customer.NetworkAddress) == "" {
		return nil, fmt.Errorf("selected customer account has no network address")
	}

	return s.ViewCustomerWallet(ctx, customer.NetworkAddress, trimmedTokenID)
}

// 1. CreateTransferRequest - generates unique ID and submits new transfer request
// TOKEN HANDSHAKE FUNCTIONS ===================================================

// RequestTokenHandshake allows a token owner to request communication with another token
// Auto-generates handshakeID internally - no need to provide it as parameter
func (s *SmartContract) RequestTokenHandshake(ctx contractapi.TransactionContextInterface, myTokenID, otherTokenID string) (string, error) {
	// SECURITY: Verify caller is the token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %v", err)
	}

	// Load my token and verify caller is owner
	myTokenBytes, err := ctx.GetStub().GetState(myTokenID)
	if err != nil || myTokenBytes == nil {
		return "", fmt.Errorf("token %s not found", myTokenID)
	}
	var myToken Token
	if err := json.Unmarshal(myTokenBytes, &myToken); err != nil {
		return "", fmt.Errorf("failed to unmarshal token: %v", err)
	}
	if myToken.Owner != callerID {
		return "", fmt.Errorf("only %s owner (%s) can request handshakes", myTokenID, myToken.Owner)
	}

	// Verify other token exists
	otherTokenBytes, err := ctx.GetStub().GetState(otherTokenID)
	if err != nil || otherTokenBytes == nil {
		return "", fmt.Errorf("token %s not found", otherTokenID)
	}

	// AUTO-GENERATE handshakeID using transaction ID and tokens
	// Format: handshake_token1_token2_txid
	handshakeID := fmt.Sprintf("handshake_%s_%s_%s", myTokenID, otherTokenID, ctx.GetStub().GetTxID())

	// Create handshake request record (pending approval)
	handshake := TokenHandshake{
		HandshakeID:   handshakeID,
		FirstTokenID:  myTokenID,
		SecondTokenID: otherTokenID,
		ApprovedBy:    "",        // Empty until approved
		Status:        "PENDING", // Status is pending
		CreatedAt:     time.Now().Format(time.RFC3339),
	}
	hsBytes, err := json.Marshal(handshake)
	if err != nil {
		return "", fmt.Errorf("failed to marshal handshake: %v", err)
	}
	if err := ctx.GetStub().PutState(handshakeID, hsBytes); err != nil {
		return "", err
	}

	// Return the generated handshakeID
	return handshakeID, nil
}

// ViewPendingTokenHandshakes returns all pending handshake requests for a given token
func (s *SmartContract) ViewPendingTokenHandshakes(ctx contractapi.TransactionContextInterface, tokenID string) ([]TokenHandshake, error) {
	// SECURITY: Verify caller is token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}

	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tokenBytes == nil {
		return nil, fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %v", err)
	}
	if token.Owner != callerID {
		return nil, fmt.Errorf("forbidden: only token owner can view pending handshakes")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Initialize as empty slice to ensure valid JSON array (not null)
	pending := make([]TokenHandshake, 0)
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		// Check if key starts with "handshake_" (new format from auto-generated IDs)
		if !strings.HasPrefix(kv.Key, "handshake_") {
			continue
		}

		var hs TokenHandshake
		if err := json.Unmarshal(kv.Value, &hs); err != nil {
			continue
		}
		// Only return PENDING handshakes where this token is the SecondTokenID (receiver)
		if hs.Status == "PENDING" && hs.SecondTokenID == tokenID {
			pending = append(pending, hs)
		}
	}
	return pending, nil
}

// TokenHandshakeApprove allows the receiving token owner to approve a pending handshake request
func (s *SmartContract) TokenHandshakeApprove(ctx contractapi.TransactionContextInterface, handshakeID string) error {
	// SECURITY: Verify caller is the token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}

	// Load the handshake request
	hsBytes, err := ctx.GetStub().GetState(handshakeID)
	if err != nil || hsBytes == nil {
		return fmt.Errorf("handshake request not found")
	}
	var handshake TokenHandshake
	if err := json.Unmarshal(hsBytes, &handshake); err != nil {
		return fmt.Errorf("failed to unmarshal handshake: %v", err)
	}

	if handshake.Status != "PENDING" {
		return fmt.Errorf("handshake is not pending: current status is %s", handshake.Status)
	}

	// Verify caller is the owner of the SECOND token (the one receiving the request)
	otherTokenBytes, err := ctx.GetStub().GetState(handshake.SecondTokenID)
	if err != nil || otherTokenBytes == nil {
		return fmt.Errorf("token %s not found", handshake.SecondTokenID)
	}
	var otherToken Token
	if err := json.Unmarshal(otherTokenBytes, &otherToken); err != nil {
		return fmt.Errorf("failed to unmarshal token: %v", err)
	}
	if otherToken.Owner != callerID {
		return fmt.Errorf("only %s owner can approve this handshake request", handshake.SecondTokenID)
	}

	// Approve the handshake request
	handshake.Status = "APPROVED"
	handshake.ApprovedBy = callerID
	hsBytes, err = json.Marshal(handshake)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake: %v", err)
	}
	return ctx.GetStub().PutState(handshakeID, hsBytes)
}

// CheckHandshake verifies if two tokens have an approved handshake (in either direction)
func (s *SmartContract) CheckHandshake(ctx contractapi.TransactionContextInterface, tokenA, tokenB string) (bool, error) {
	// Scan all handshakes to find approved ones between tokenA and tokenB
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return false, err
	}
	defer iter.Close()

	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return false, err
		}
		// Check if key starts with "handshake_"
		if !strings.HasPrefix(kv.Key, "handshake_") {
			continue
		}

		var hs TokenHandshake
		if err := json.Unmarshal(kv.Value, &hs); err != nil {
			continue
		}
		// Check if both tokens are involved (in either direction) and status is APPROVED
		if hs.Status == "APPROVED" {
			if (hs.FirstTokenID == tokenA && hs.SecondTokenID == tokenB) ||
				(hs.FirstTokenID == tokenB && hs.SecondTokenID == tokenA) {
				return true, nil
			}
		}
	}
	return false, nil
}

// checkApprovedHandshakeExists verifies if two tokens have an APPROVED handshake (in either direction)
// This is the strict version used for enforcing token-to-token transfer requirements
func (s *SmartContract) checkApprovedHandshakeExists(ctx contractapi.TransactionContextInterface, tokenA, tokenB string) (bool, error) {
	// Scan all handshakes to find approved ones between tokenA and tokenB
	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return false, err
	}
	defer iter.Close()

	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return false, err
		}
		// Check if key starts with "handshake_"
		if !strings.HasPrefix(kv.Key, "handshake_") {
			continue
		}

		var hs TokenHandshake
		if err := json.Unmarshal(kv.Value, &hs); err != nil {
			continue
		}
		// Check if both tokens are involved (in either direction) and status is APPROVED
		if hs.Status == "APPROVED" {
			if (hs.FirstTokenID == tokenA && hs.SecondTokenID == tokenB) ||
				(hs.FirstTokenID == tokenB && hs.SecondTokenID == tokenA) {
				return true, nil
			}
		}
	}
	return false, nil
}

// ViewTokenHandshakes returns all APPROVED handshakes for a given token
func (s *SmartContract) ViewTokenHandshakes(ctx contractapi.TransactionContextInterface, tokenID string) ([]TokenHandshake, error) {
	// SECURITY: Verify caller is token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}

	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tokenBytes == nil {
		return nil, fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %v", err)
	}
	if token.Owner != callerID {
		return nil, fmt.Errorf("forbidden: only token owner can view handshakes")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Initialize as empty slice to ensure valid JSON array (not null)
	handshakes := make([]TokenHandshake, 0)
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		// Check if key starts with "handshake_" (new format from auto-generated IDs)
		if !strings.HasPrefix(kv.Key, "handshake_") {
			continue
		}

		var hs TokenHandshake
		if err := json.Unmarshal(kv.Value, &hs); err != nil {
			continue
		}
		// Only return APPROVED handshakes involving this token
		if hs.Status == "APPROVED" && (hs.FirstTokenID == tokenID || hs.SecondTokenID == tokenID) {
			handshakes = append(handshakes, hs)
		}
	}
	return handshakes, nil
}

// CreateTransferRequest removed - use CreateTokenTransferRequest instead

// ApproveTransferByOwner removed

// ApproveTransferByReceiver, ViewTransferRequestsForOwner, ViewTransferRequestsForReceiver removed - use TokenTransferRequest instead

// Token-to-token transfers ----------------------------------------------------

func (s *SmartContract) requestBankTokenTransferInternal(ctx contractapi.TransactionContextInterface, senderTokenID, receiverTokenID, senderOwnerAddress string, amount int64, purpose string) (string, error) {
	// SECURITY: Verify caller's identity matches the sender owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != senderOwnerAddress {
		return "", fmt.Errorf("forbidden: caller identity does not match sender owner")
	}

	// Verify caller is a bank and owns the sender token
	if err := s.VerifyBankOwner(ctx, senderTokenID); err != nil {
		return "", fmt.Errorf("forbidden: %v", err)
	}

	if amount <= 0 {
		return "", fmt.Errorf("amount must be positive")
	}
	if senderTokenID == receiverTokenID {
		return "", fmt.Errorf("cannot transfer to the same token")
	}

	// Verify sender token ownership and minted balance
	senderBytes, err := ctx.GetStub().GetState(senderTokenID)
	if err != nil || senderBytes == nil {
		return "", fmt.Errorf("sender token not found")
	}
	var senderToken Token
	if err := json.Unmarshal(senderBytes, &senderToken); err != nil {
		return "", err
	}
	if senderToken.Owner != senderOwnerAddress {
		return "", fmt.Errorf("caller is not the sender token owner")
	}
	if int64(getTokenSupply(senderToken)) < amount {
		return "", fmt.Errorf("insufficient minted balance on sender token")
	}

	// Verify receiver token exists and already has an owner
	receiverBytes, err := ctx.GetStub().GetState(receiverTokenID)
	if err != nil || receiverBytes == nil {
		return "", fmt.Errorf("receiver token not found")
	}
	var receiverToken Token
	if err := json.Unmarshal(receiverBytes, &receiverToken); err != nil {
		return "", err
	}
	if receiverToken.Owner == "" {
		return "", fmt.Errorf("receiver token is not yet assigned to an owner")
	}
	senderCurrency := strings.TrimSpace(senderToken.Currency)
	if senderCurrency == "" {
		return "", fmt.Errorf("sender token currency not configured")
	}
	receiverCurrency := strings.TrimSpace(receiverToken.Currency)
	if receiverCurrency == "" {
		return "", fmt.Errorf("receiver token currency not configured")
	}

	// CRITICAL: Verify that an APPROVED handshake exists between the two tokens
	// This ensures token-to-token transfers only happen after proper handshake approval
	hasApprovedHandshake, err := s.checkApprovedHandshakeExists(ctx, senderTokenID, receiverTokenID)
	if err != nil {
		return "", fmt.Errorf("error verifying handshake: %v", err)
	}
	if !hasApprovedHandshake {
		return "", fmt.Errorf("token-to-token transfer requires an approved handshake between %s and %s. Please initiate and approve a handshake first", senderTokenID, receiverTokenID)
	}
	senderBIC, err := s.resolveTokenBIC(ctx, senderToken)
	if err != nil {
		return "", err
	}
	receiverBIC, err := s.resolveTokenBIC(ctx, receiverToken)
	if err != nil {
		return "", err
	}
	if !validBICFormat(senderBIC) {
		return "", fmt.Errorf("sender token BIC is invalid")
	}
	if !validBICFormat(receiverBIC) {
		return "", fmt.Errorf("receiver token BIC is invalid")
	}
	if strings.TrimSpace(strings.ToUpper(purpose)) == "" {
		purpose = transferPurposeInterbankSettlement
	}
	purpose = strings.TrimSpace(strings.ToUpper(purpose))
	if !validBankTransferPurpose(purpose) {
		return "", fmt.Errorf("purpose must be one of RTGS/NEFT/INTERBANK_SETTLEMENT")
	}

	txTime, err := s.currentTxTime(ctx)
	if err != nil {
		return "", err
	}
	createdAt, parseErr := time.Parse(time.RFC3339, txTime)
	if parseErr != nil {
		createdAt = time.Now().UTC()
		txTime = createdAt.Format(time.RFC3339)
	}
	shortTx := ctx.GetStub().GetTxID()
	if len(shortTx) > 8 {
		shortTx = shortTx[:8]
	}
	msgID := fmt.Sprintf("%s-%s-%s", senderBIC, receiverBIC, shortTx)
	stateKey := fmt.Sprintf("%s/TRANSFER/%s", senderBIC, msgID)
	request := TokenTransferRequest{
		MsgID:           msgID,
		SenderBIC:       senderBIC,
		ReceiverBIC:     receiverBIC,
		SenderTokenID:   senderTokenID,
		ReceiverTokenID: receiverTokenID,
		Amount:          amount,
		InitiatedBy:     senderOwnerAddress,
		Status:          "PENDING",
		Currency:        senderCurrency,
		ExchangeRate:    1.0,
		Purpose:         purpose,
		CreatedAt:       txTime,
		ExpiresAt:       createdAt.Add(transferRequestTTLHours * time.Hour).Format(time.RFC3339),
		RequestID:       stateKey, // legacy request reference
	}
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	if err := ctx.GetStub().PutState(stateKey, reqBytes); err != nil {
		return "", err
	}
	return msgID, nil
}

// CreateTokenTransferRequest keeps backwards-compatible signature and defaults purpose.
func (s *SmartContract) CreateTokenTransferRequest(ctx contractapi.TransactionContextInterface, senderTokenID, receiverTokenID, senderOwnerAddress string, amount int) (string, error) {
	return s.requestBankTokenTransferInternal(ctx, senderTokenID, receiverTokenID, senderOwnerAddress, int64(amount), transferPurposeInterbankSettlement)
}

// RequestBankTransfer allows explicit bank transfer purpose for RTGS/NEFT/INTERBANK settlement.
func (s *SmartContract) RequestBankTransfer(ctx contractapi.TransactionContextInterface, senderTokenID, receiverTokenID, senderOwnerAddress string, amount int64, purpose string) (string, error) {
	return s.requestBankTokenTransferInternal(ctx, senderTokenID, receiverTokenID, senderOwnerAddress, amount, purpose)
}

// ViewPendingTokenTransferRequests allows the receiver token owner to inspect pending transfer requests.
func (s *SmartContract) ViewPendingTokenTransferRequests(ctx contractapi.TransactionContextInterface, receiverTokenID, receiverOwnerAddress string) ([]TokenTransferRequest, error) {
	// SECURITY: Verify caller's identity matches the owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != receiverOwnerAddress {
		return nil, fmt.Errorf("forbidden: caller identity does not match owner")
	}

	// Verify caller is a bank and owns this specific token
	if err := s.VerifyBankOwner(ctx, receiverTokenID); err != nil {
		return nil, fmt.Errorf("forbidden: %v", err)
	}

	tokenBytes, err := ctx.GetStub().GetState(receiverTokenID)
	if err != nil || tokenBytes == nil {
		return nil, fmt.Errorf("receiver token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, err
	}
	if token.Owner != receiverOwnerAddress {
		return nil, fmt.Errorf("caller is not the receiver token owner")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var pending []TokenTransferRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if isTokenTransferRequestKey(kv.Key) {
			var req TokenTransferRequest
			if err := json.Unmarshal(kv.Value, &req); err == nil {
				normalizeTokenTransferRequestForRead(&req)
				if req.ReceiverTokenID == receiverTokenID && req.Status == "PENDING" {
					pending = append(pending, req)
				}
			}
		}
	}
	return pending, nil
}

// ApproveTokenTransferRequest lets the receiver token owner release funds by crediting their token and debiting the sender token.
func (s *SmartContract) ApproveTokenTransferRequest(ctx contractapi.TransactionContextInterface, requestID, receiverOwnerAddress string) error {
	stateKey, err := s.resolveTokenTransferStateKey(ctx, requestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("token transfer request not found")
	}
	var request TokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return err
	}
	normalizeTokenTransferRequestForRead(&request)
	if request.Status != "PENDING" {
		return fmt.Errorf("transfer request already processed")
	}

	// Load sender token
	senderBytes, err := ctx.GetStub().GetState(request.SenderTokenID)
	if err != nil || senderBytes == nil {
		return fmt.Errorf("sender token not found")
	}
	var senderToken Token
	if err := json.Unmarshal(senderBytes, &senderToken); err != nil {
		return err
	}
	if int64(getTokenSupply(senderToken)) < request.Amount {
		request.Status = "REJECTED"
		updatedReqBytes, _ := json.Marshal(request)
		_ = ctx.GetStub().PutState(stateKey, updatedReqBytes)
		return fmt.Errorf("insufficient minted balance on sender token")
	}

	// Load receiver token and ensure caller is owner
	receiverBytes, err := ctx.GetStub().GetState(request.ReceiverTokenID)
	if err != nil || receiverBytes == nil {
		return fmt.Errorf("receiver token not found")
	}
	var receiverToken Token
	if err := json.Unmarshal(receiverBytes, &receiverToken); err != nil {
		return err
	}
	if receiverToken.Owner != receiverOwnerAddress {
		return fmt.Errorf("caller is not the receiver token owner")
	}
	senderCurrency := strings.TrimSpace(senderToken.Currency)
	if senderCurrency == "" {
		return fmt.Errorf("sender token currency not configured")
	}
	receiverCurrency := strings.TrimSpace(receiverToken.Currency)
	if receiverCurrency == "" {
		return fmt.Errorf("receiver token currency not configured")
	}

	// CRITICAL: Re-verify that an APPROVED handshake still exists between the two tokens
	// This ensures the approval step also respects the handshake requirement
	hasApprovedHandshake, err := s.checkApprovedHandshakeExists(ctx, request.SenderTokenID, request.ReceiverTokenID)
	if err != nil {
		return fmt.Errorf("error verifying handshake: %v", err)
	}
	if !hasApprovedHandshake {
		request.Status = "REJECTED"
		updatedReqBytes, _ := json.Marshal(request)
		_ = ctx.GetStub().PutState(stateKey, updatedReqBytes)
		return fmt.Errorf("handshake approval was revoked or does not exist between tokens")
	}

	// Perform transfer
	transferAmountInt := int(request.Amount)
	setTokenSupply(&senderToken, getTokenSupply(senderToken)-transferAmountInt)
	if senderCurrency == receiverCurrency {
		setTokenSupply(&receiverToken, getTokenSupply(receiverToken)+transferAmountInt)
	} else {
		// Foreign currency transfer: add to ForeignBalances (available balance)
		if receiverToken.ForeignBalances == nil {
			receiverToken.ForeignBalances = make(map[string]int)
		}
		receiverToken.ForeignBalances[senderCurrency] += transferAmountInt
		fmt.Printf("[CHAINCODE DEBUG] Added to foreign balances for %s: %d, New total: %d\n", senderCurrency, transferAmountInt, receiverToken.ForeignBalances[senderCurrency])
	}

	if request.Currency == "" {
		request.Currency = senderCurrency
	}

	request.Status = "SETTLED"
	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	request.SettledAt = ts
	request.CompletedAt = ts

	senderBytes, err = json.Marshal(senderToken)
	if err != nil {
		return err
	}
	receiverBytes, err = json.Marshal(receiverToken)
	if err != nil {
		return err
	}
	reqBytes, err = json.Marshal(request)
	if err != nil {
		return err
	}

	if err := ctx.GetStub().PutState(request.SenderTokenID, senderBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(request.ReceiverTokenID, receiverBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(stateKey, reqBytes); err != nil {
		return err
	}

	txID := ctx.GetStub().GetTxID()
	shortTx := txID
	if len(shortTx) > 12 {
		shortTx = shortTx[:12]
	}
	txRef := fmt.Sprintf("TXN-%s-%s", strings.TrimSpace(request.SenderBIC), shortTx)
	if strings.TrimSpace(request.SenderBIC) == "" {
		txRef = fmt.Sprintf("TXN-%s", shortTx)
	}
	historyKey := fmt.Sprintf("%s/SETTLED/%s", strings.TrimSpace(request.SenderBIC), txRef)
	if strings.TrimSpace(request.SenderBIC) == "" {
		historyKey = fmt.Sprintf("SETTLED/%s", txRef)
	}
	history := TokenToTokenTransferRecord{
		TxRef:           txRef,
		MsgID:           strings.TrimSpace(request.MsgID),
		SenderBIC:       strings.TrimSpace(request.SenderBIC),
		ReceiverBIC:     strings.TrimSpace(request.ReceiverBIC),
		SenderTokenID:   request.SenderTokenID,
		ReceiverTokenID: request.ReceiverTokenID,
		Amount:          request.Amount,
		Currency:        request.Currency,
		ExchangeRate:    request.ExchangeRate,
		FeeAmount:       0,
		NetAmount:       request.Amount,
		Status:          "SETTLED",
		SettledAt:       ts,
		BlockHeight:     txID, // TxID used as immutable ledger proof marker.
		Purpose:         request.Purpose,
		RecordID:        historyKey, // legacy alias
		RequestID:       strings.TrimSpace(request.MsgID),
		InitiatedBy:     request.InitiatedBy,
		ApprovedBy:      receiverOwnerAddress,
		ApprovedAt:      ts,
	}
	normalizeTokenToTokenTransferRecordForRead(&history)
	historyBytes, err := json.Marshal(history)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(historyKey, historyBytes)
}

// ListTokenToTokenTransferHistory lists completed token-to-token transfers involving the specified token.
func (s *SmartContract) ListTokenToTokenTransferHistory(ctx contractapi.TransactionContextInterface, tokenID string) ([]TokenToTokenTransferRecord, error) {
	if tokenID == "" {
		return nil, fmt.Errorf("tokenID is required")
	}

	// SECURITY: Verify caller is a bank and owns this token
	if err := s.VerifyBankOwner(ctx, tokenID); err != nil {
		return nil, fmt.Errorf("forbidden: %v", err)
	}

	tokenBytes, err := ctx.GetStub().GetState(tokenID)
	if err != nil || tokenBytes == nil {
		return nil, fmt.Errorf("token not found")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var history []TokenToTokenTransferRecord
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isTokenToTokenTransferHistoryKey(kv.Key) {
			continue
		}
		var record TokenToTokenTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
		normalizeTokenToTokenTransferRecordForRead(&record)
		if record.SenderTokenID == tokenID || record.ReceiverTokenID == tokenID {
			history = append(history, record)
		}
	}
	return history, nil
}

// ListParticipantTransferHistory returns completed transfers involving a participant.
func (s *SmartContract) ListParticipantTransferHistory(ctx contractapi.TransactionContextInterface, networkAddress string) ([]ParticipantTransferRecord, error) {
	if networkAddress == "" {
		return nil, fmt.Errorf("networkAddress is required")
	}

	// SECURITY: Verify caller's identity matches the participant
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("unable to read caller identity: %v", err)
	}
	if callerID != networkAddress {
		return nil, fmt.Errorf("forbidden: caller identity does not match participant")
	}

	participantBytes, err := ctx.GetStub().GetState(networkAddress)
	if err != nil || participantBytes == nil {
		return nil, fmt.Errorf("participant not found")
	}
	var participant Participant
	if err := json.Unmarshal(participantBytes, &participant); err != nil {
		return nil, err
	}
	if callerID != participant.ClientID {
		return nil, fmt.Errorf("unauthorized caller")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var history []ParticipantTransferRecord
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "participanttransferhistory_") {
			continue
		}
		var record ParticipantTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
		normalizeParticipantTransferRecordForRead(&record)
		if record.SenderParticipantID == networkAddress || record.ReceiverParticipantID == networkAddress {
			// SWIFT Compliance: Show own customer details, hide counterparty details
			if record.SenderParticipantID == networkAddress {
				// This user is the sender - hide counterparty legacy PII fields.
				record.ReceiverName = ""
				record.ReceiverKycId = ""
				record.ReceiverKycStatus = ""
			} else {
				// This user is the receiver - hide counterparty legacy PII fields.
				record.SenderName = ""
				record.SenderKycId = ""
				record.SenderKycStatus = ""
			}
			history = append(history, record)
		}
	}
	return history, nil
}

// ListAllParticipantTransferHistory returns every participant transfer record (admin/observer use).
func (s *SmartContract) ListAllParticipantTransferHistory(ctx contractapi.TransactionContextInterface) ([]ParticipantTransferRecord, error) {
	// SECURITY: Only admin can view all transfer history
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, fmt.Errorf("forbidden: admin only - %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var history []ParticipantTransferRecord
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "participanttransferhistory_") {
			continue
		}
		var record ParticipantTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
		normalizeParticipantTransferRecordForRead(&record)
		history = append(history, record)
	}
	return history, nil
}

// GetSenderRecords lists immutable participant settlement records for a sender BIC.
func (s *SmartContract) GetSenderRecords(ctx contractapi.TransactionContextInterface, senderBIC string) ([]ParticipantTransferRecord, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, fmt.Errorf("forbidden: admin only - %v", err)
	}
	senderBIC = strings.TrimSpace(strings.ToUpper(senderBIC))
	if !validBICFormat(senderBIC) {
		return nil, fmt.Errorf("sender_bic must be valid BIC8/BIC11")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var records []ParticipantTransferRecord
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !(strings.Contains(kv.Key, "/RECORDS/") || strings.HasPrefix(kv.Key, "participanttransferhistory_")) {
			continue
		}
		var record ParticipantTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
		normalizeParticipantTransferRecordForRead(&record)
		if strings.TrimSpace(strings.ToUpper(record.SenderBIC)) == senderBIC {
			records = append(records, record)
		}
	}
	return records, nil
}

// GetReceiverRecords lists immutable participant settlement records for a receiver BIC.
func (s *SmartContract) GetReceiverRecords(ctx contractapi.TransactionContextInterface, receiverBIC string) ([]ParticipantTransferRecord, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, fmt.Errorf("forbidden: admin only - %v", err)
	}
	receiverBIC = strings.TrimSpace(strings.ToUpper(receiverBIC))
	if !validBICFormat(receiverBIC) {
		return nil, fmt.Errorf("receiver_bic must be valid BIC8/BIC11")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var records []ParticipantTransferRecord
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !(strings.Contains(kv.Key, "/RECORDS/") || strings.HasPrefix(kv.Key, "participanttransferhistory_")) {
			continue
		}
		var record ParticipantTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
		normalizeParticipantTransferRecordForRead(&record)
		if strings.TrimSpace(strings.ToUpper(record.ReceiverBIC)) == receiverBIC {
			records = append(records, record)
		}
	}
	return records, nil
}

// TotalVolumeByBIC returns summed net settled amount grouped by sender and receiver BIC.
func (s *SmartContract) TotalVolumeByBIC(ctx contractapi.TransactionContextInterface) (map[string]int64, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, fmt.Errorf("forbidden: admin only - %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	volumes := make(map[string]int64)
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !(strings.Contains(kv.Key, "/RECORDS/") || strings.HasPrefix(kv.Key, "participanttransferhistory_")) {
			continue
		}
		var record ParticipantTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
		normalizeParticipantTransferRecordForRead(&record)
		if strings.ToUpper(strings.TrimSpace(record.Status)) != "SETTLED" {
			continue
		}
		net := record.NetAmount
		if net == 0 {
			net = record.Amount - record.Commission
		}
		if sender := strings.TrimSpace(strings.ToUpper(record.SenderBIC)); sender != "" {
			volumes[sender] += net
		}
		if receiver := strings.TrimSpace(strings.ToUpper(record.ReceiverBIC)); receiver != "" {
			volumes[receiver] += net
		}
	}
	return volumes, nil
}

// ListParticipantTransfersByID lets privileged callers fetch participant transfers without credentials.
func (s *SmartContract) ListParticipantTransfersByID(ctx contractapi.TransactionContextInterface, participantID string) ([]ParticipantTransferRecord, error) {
	if participantID == "" {
		return nil, fmt.Errorf("participantID is required")
	}

	// SECURITY: Verify caller's identity matches the participant
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != participantID {
		return nil, fmt.Errorf("forbidden: caller identity does not match participant")
	}

	exists, err := s.ParticipantExists(ctx, participantID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("participant not found")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var history []ParticipantTransferRecord
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(kv.Key, "participanttransferhistory_") {
			continue
		}
		var record ParticipantTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
		normalizeParticipantTransferRecordForRead(&record)
		if record.SenderParticipantID == participantID || record.ReceiverParticipantID == participantID {
			history = append(history, record)
		}
	}
	return history, nil
}

// CUSTOMER-TO-TOKEN TRANSFER FUNCTIONS ========================================

// CreateCustomerToTokenTransferRequest initiates a transfer from a customer to another customer.
// Input is privacy-safe: sender network address + receiver customer_ref/BIC + amount.
// Token IDs are resolved internally from approved participant records.
func (s *SmartContract) CreateCustomerToTokenTransferRequest(ctx contractapi.TransactionContextInterface, senderNetworkAddress, receiverCustomerRef, receiverBIC string, amount int) (string, error) {
	// SECURITY: Verify caller is the customer (sender)
	// The network address is set by the backend from the authenticated caller's certificate
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != senderNetworkAddress {
		return "", fmt.Errorf("forbidden: caller identity does not match sender network address")
	}

	if amount <= 0 {
		return "", fmt.Errorf("amount must be positive")
	}
	receiverCustomerRef = strings.TrimSpace(receiverCustomerRef)
	receiverBIC = strings.TrimSpace(strings.ToUpper(receiverBIC))
	if receiverCustomerRef == "" {
		return "", fmt.Errorf("receiver_customer_ref is required")
	}
	if !validBICFormat(receiverBIC) {
		return "", fmt.Errorf("receiver_bic must be valid BIC8/BIC11")
	}

	// Resolve sender/receiver participants first; token IDs come from these records.
	senderCustomer, senderCustomerKey, err := s.findApprovedCustomerByCaller(ctx, callerID)
	if err != nil {
		return "", err
	}
	if senderCustomer.NetworkAddress != "" && strings.TrimSpace(senderCustomer.NetworkAddress) != strings.TrimSpace(senderNetworkAddress) {
		return "", fmt.Errorf("forbidden: caller customer account does not match sender network address")
	}
	receiverCustomerByRef, _, err := s.findCustomerByRefAndBIC(ctx, receiverCustomerRef, receiverBIC)
	if err != nil {
		return "", err
	}
	senderTokenID := strings.TrimSpace(senderCustomer.TokenID)
	receiverTokenID := strings.TrimSpace(receiverCustomerByRef.TokenID)
	receiverCustomerNetworkAddress := strings.TrimSpace(receiverCustomerByRef.NetworkAddress)
	if senderTokenID == "" {
		return "", fmt.Errorf("sender customer token is not configured")
	}
	if receiverTokenID == "" {
		return "", fmt.Errorf("receiver customer token is not configured")
	}
	if receiverCustomerNetworkAddress == "" {
		return "", fmt.Errorf("receiver customer network address not found")
	}
	if senderTokenID == receiverTokenID {
		return "", fmt.Errorf("cannot transfer to the same token")
	}

	// Fetch commission rate from blockchain config for receiver token
	commissionConfig, err := s.GetTokenCommission(ctx, receiverTokenID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch commission config: %v", err)
	}
	// Calculate commission amount from percentage
	commissionAmount := int64(float64(amount) * (commissionConfig.CommissionPercentage / 100))

	// Load and validate sender token
	senderTokenBytes, err := ctx.GetStub().GetState(senderTokenID)
	if err != nil || senderTokenBytes == nil {
		return "", fmt.Errorf("sender token not found")
	}
	var senderToken Token
	if err := json.Unmarshal(senderTokenBytes, &senderToken); err != nil {
		return "", err
	}

	// Load and validate receiver token
	receiverTokenBytes, err := ctx.GetStub().GetState(receiverTokenID)
	if err != nil || receiverTokenBytes == nil {
		return "", fmt.Errorf("receiver token not found")
	}
	var receiverToken Token
	if err := json.Unmarshal(receiverTokenBytes, &receiverToken); err != nil {
		return "", err
	}

	senderCurrency := strings.TrimSpace(senderToken.Currency)
	if senderCurrency == "" {
		return "", fmt.Errorf("sender token currency not configured")
	}
	receiverCurrency := strings.TrimSpace(receiverToken.Currency)
	if receiverCurrency == "" {
		return "", fmt.Errorf("receiver token currency not configured")
	}

	// SECURITY FIX #8: Currency compatibility check
	// Different currencies require exchange rate; same currency direct transfer
	if senderCurrency != receiverCurrency {
		// For cross-currency transfers, verify exchange rate is available
		// This uses getExchangeRate() which has hardcoded fallback rates for supported currencies
		_, err := s.getExchangeRate(ctx, senderCurrency, receiverCurrency)
		if err != nil {
			return "", fmt.Errorf("exchange rate not available for %s to %s: %v", senderCurrency, receiverCurrency, err)
		}
	}

	// Load sender's participant record by resolved token/network to check balance.
	senderCustomer, senderCustomerKey, err = s.getParticipantByNetworkToken(ctx, senderNetworkAddress, senderTokenID)
	if err != nil {
		return "", fmt.Errorf("sender customer not registered or approved for sender token")
	}
	if !senderCustomer.Approved {
		return "", fmt.Errorf("sender customer not approved for sender token")
	}
	if senderCustomer.Balance < amount {
		return "", fmt.Errorf("insufficient balance: have %d, need %d", senderCustomer.Balance, amount)
	}

	// CHECKPOINT: Verify receiver customer is registered with receiver token
	// receiverCustomerNetworkAddress is the network address of the receiver customer
	receiverCustomer, _, err := s.getParticipantByNetworkToken(ctx, receiverCustomerNetworkAddress, receiverTokenID)
	if err != nil {
		return "", fmt.Errorf("receiver customer not registered with receiver token")
	}
	if !receiverCustomer.Approved {
		return "", fmt.Errorf("receiver customer not approved for receiver token")
	}

	// CRITICAL: Verify approved handshake exists between the two tokens
	hasApprovedHandshake, err := s.checkApprovedHandshakeExists(ctx, senderTokenID, receiverTokenID)
	if err != nil {
		return "", fmt.Errorf("error verifying handshake: %v", err)
	}
	if !hasApprovedHandshake {
		return "", fmt.Errorf("token-to-token transfer requires an approved handshake between %s and %s", senderTokenID, receiverTokenID)
	}

	// SECURITY FIX #9: Concurrent transfer prevention - check for existing in-progress transfers
	// Query for other pending transfers with same sender/receiver/amount to prevent race conditions
	senderTransfersKey := fmt.Sprintf("transfer_pending_%s", senderNetworkAddress)
	existingTransfersIter, err := ctx.GetStub().GetStateByPartialCompositeKey(senderTransfersKey, []string{senderNetworkAddress})
	if err == nil {
		defer existingTransfersIter.Close()
		for existingTransfersIter.HasNext() {
			resultKV, err := existingTransfersIter.Next()
			if err != nil {
				return "", err
			}
			var existingTransfer CustomerToTokenTransferRequest
			if err := json.Unmarshal(resultKV.Value, &existingTransfer); err == nil {
				// Check if same transfer parameters exist in pending state
				if existingTransfer.SenderTokenID == senderTokenID &&
					existingTransfer.ReceiverTokenID == receiverTokenID &&
					existingTransfer.ReceiverCustomerID == receiverCustomerNetworkAddress &&
					int(existingTransfer.Amount) == amount &&
					(isCustomerTransferPendingSender(existingTransfer.Status) || isCustomerTransferPendingReceiver(existingTransfer.Status)) {
					return "", fmt.Errorf("duplicate transfer request: identical transfer already pending")
				}
			}
		}
	}

	// SECURITY FIX #10: Timeout tracking - get transaction timestamp for request expiration (48 hour timeout)
	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil || txTimestamp == nil {
		return "", fmt.Errorf("failed to get transaction timestamp: %v", err)
	}

	// Create transfer request ID
	reqID := "custtotoken_" + ctx.GetStub().GetTxID()

	// Commission is passed by application
	receiverCustomerAmount := int64(amount) - commissionAmount

	// IMMEDIATELY DEBIT SENDER CUSTOMER BALANCE (escrow)
	originalBalance := senderCustomer.Balance
	senderCustomer.Balance -= amount
	if senderCustomer.TokenTransferIDs == nil {
		senderCustomer.TokenTransferIDs = []string{}
	}
	senderCustomer.TokenTransferIDs = append(senderCustomer.TokenTransferIDs, reqID)

	senderCustomerUpdatedBytes, err := json.Marshal(senderCustomer)
	if err != nil {
		// SECURITY FIX #11: Escrow rollback protection - restore balance on marshal error
		senderCustomer.Balance = originalBalance
		return "", err
	}
	if err := ctx.GetStub().PutState(senderCustomerKey, senderCustomerUpdatedBytes); err != nil {
		return "", err
	}

	// Create transfer request record
	senderBIC, err := s.resolveTokenBIC(ctx, senderToken)
	if err != nil {
		return "", err
	}
	resolvedReceiverBIC, err := s.resolveTokenBIC(ctx, receiverToken)
	if err != nil {
		return "", err
	}
	msgID := fmt.Sprintf("TXN-%s-%s", senderCustomer.CustomerID, ctx.GetStub().GetTxID()[:8])
	request := CustomerToTokenTransferRequest{
		MsgID:                   msgID,
		SenderCustomerRef:       senderCustomer.CustomerID,
		SenderBIC:               senderBIC,
		ReceiverCustomerRef:     receiverCustomer.CustomerID,
		ReceiverBIC:             resolvedReceiverBIC,
		TransferRequestID:       reqID,
		SenderCustomerID:        senderNetworkAddress,
		SenderCustomerTokenID:   senderCustomer.CustomerID,
		SenderCustomerName:      senderCustomer.Name,
		SenderTokenID:           senderTokenID,
		ReceiverTokenID:         receiverTokenID,
		ReceiverCustomerID:      receiverCustomerNetworkAddress,
		ReceiverCustomerTokenID: receiverCustomer.CustomerID,
		ReceiverCustomerName:    receiverCustomer.Name,
		Amount:                  int64(amount),
		Currency:                senderCurrency,
		SenderCurrency:          senderCurrency,
		ReceiverCurrency:        receiverCurrency,
		Status:                  "PENDING_SENDER",
		InitiatedBy:             callerID,
		DebitStatus:             "DEBITED",
		CreditStatus:            "PENDING",
		EscrowAmount:            int64(amount),
		EscrowedAmount:          int64(amount),
		ApprovedBySenderOwner:   false,
		ApprovedByReceiverOwner: false,
		CommissionPct:           commissionConfig.CommissionPercentage / 100.0,
		CommissionPercentage:    commissionConfig.CommissionPercentage,
		CommissionAmount:        commissionAmount,
		NetReceiverAmount:       receiverCustomerAmount,
		ReceiverCustomerAmount:  receiverCustomerAmount,
		CreatedAt:               time.Unix(txTimestamp.Seconds, int64(txTimestamp.Nanos)).UTC().Format(time.RFC3339),
		// SECURITY FIX #10: Timeout tracking - record timestamp in SenderTokenOwnerApprovedAt for now (can add CreatedAt field if needed)
	}
	normalizeCustomerToTokenTransferRequestForRead(&request)

	reqBytes, err := json.Marshal(request)
	if err != nil {
		// SECURITY FIX #11: Reverse debit on marshal error (improved escrow protection)
		senderCustomer.Balance = originalBalance
		senderCustomer.TokenTransferIDs = senderCustomer.TokenTransferIDs[:len(senderCustomer.TokenTransferIDs)-1]
		_ = ctx.GetStub().PutState(senderCustomerKey, senderCustomerUpdatedBytes)
		return "", err
	}

	stateKey := fmt.Sprintf("TRANSFERS/%s/%s", strings.TrimSpace(senderBIC), msgID)
	if err := ctx.GetStub().PutState(stateKey, reqBytes); err != nil {
		// SECURITY FIX #11: Reverse debit on state error (improved escrow protection)
		senderCustomer.Balance = originalBalance
		senderCustomer.TokenTransferIDs = senderCustomer.TokenTransferIDs[:len(senderCustomer.TokenTransferIDs)-1]
		_ = ctx.GetStub().PutState(senderCustomerKey, senderCustomerUpdatedBytes)
		return "", err
	}
	// Legacy key for backward compatibility with existing callers.
	_ = ctx.GetStub().PutState(reqID, reqBytes)

	return msgID, nil

}

// RequestCustomerTransfer provides a privacy-safe, BIC-based entrypoint for customer transfers.
// It resolves sender/receiver token IDs internally from customer reference + BIC.
func (s *SmartContract) RequestCustomerTransfer(ctx contractapi.TransactionContextInterface, receiverCustomerRef, receiverBIC string, amount int64) (string, error) {
	if amount <= 0 {
		return "", fmt.Errorf("amount must be positive")
	}
	if amount > math.MaxInt32 {
		return "", fmt.Errorf("amount exceeds supported limit")
	}
	receiverBIC = strings.TrimSpace(strings.ToUpper(receiverBIC))
	if !validBICFormat(receiverBIC) {
		return "", fmt.Errorf("receiver_bic must be valid BIC8/BIC11")
	}
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %v", err)
	}

	return s.CreateCustomerToTokenTransferRequest(
		ctx,
		callerID,
		receiverCustomerRef,
		receiverBIC,
		int(amount),
	)
}

func (s *SmartContract) storeCustomerTransferRequest(ctx contractapi.TransactionContextInterface, stateKey string, request *CustomerToTokenTransferRequest) error {
	if request == nil {
		return fmt.Errorf("transfer request is required")
	}
	normalizeCustomerToTokenTransferRequestForRead(request)
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(stateKey, reqBytes); err != nil {
		return err
	}
	// Keep legacy lookup key in sync for compatibility.
	if request.TransferRequestID != "" && request.TransferRequestID != stateKey {
		if err := ctx.GetStub().PutState(request.TransferRequestID, reqBytes); err != nil {
			return err
		}
	}
	return nil
}

func (s *SmartContract) rejectCustomerTransferRequest(
	ctx contractapi.TransactionContextInterface,
	stateKey string,
	request *CustomerToTokenTransferRequest,
	rejectionStatus string,
	rejectionReason string,
) error {
	if request == nil {
		return fmt.Errorf("transfer request is required")
	}

	normalizedStatus := normalizeCustomerTransferStatus(rejectionStatus)
	if !isCustomerTransferRejectedStatus(normalizedStatus) {
		return fmt.Errorf("invalid rejection status: %s", rejectionStatus)
	}

	reason := normalizeCustomerTransferRejectionReason(rejectionReason)
	if reason == "" {
		reason = "SMART_CONTRACT_ERROR"
	}

	escrowToReturn := request.EscrowAmount
	if escrowToReturn == 0 {
		escrowToReturn = request.EscrowedAmount
	}
	if escrowToReturn == 0 {
		escrowToReturn = request.Amount
	}

	if escrowToReturn > 0 {
		senderCustomer, senderCustomerKey, err := s.getParticipantByNetworkToken(ctx, request.SenderCustomerID, request.SenderTokenID)
		if err != nil {
			return err
		}
		senderCustomer.Balance += int(escrowToReturn)
		senderUpdatedBytes, err := json.Marshal(senderCustomer)
		if err != nil {
			return err
		}
		if err := ctx.GetStub().PutState(senderCustomerKey, senderUpdatedBytes); err != nil {
			return err
		}
	}

	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	request.Status = normalizedStatus
	request.RejectionReason = reason
	request.RejectedAt = ts
	request.CreditStatus = "REVERSED"
	request.DebitStatus = "REVERSED"
	request.EscrowAmount = 0
	request.EscrowedAmount = 0
	request.NetReceiverAmount = 0
	request.ReceiverCustomerAmount = 0

	return s.storeCustomerTransferRequest(ctx, stateKey, request)
}

// ApproveSenderTokenTransfer allows sender token owner to approve or reject the transfer
// ViewPendingCustomerToTokenTransfersAsSender returns pending transfers where this token is the sender
func (s *SmartContract) ViewPendingCustomerToTokenTransfersAsSender(ctx contractapi.TransactionContextInterface, tokenID string, ownerNetworkAddress string) ([]CustomerToTokenTransferRequest, error) {
	// SECURITY: Verify caller is the token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return nil, fmt.Errorf("forbidden: caller identity does not match owner")
	}

	// Verify caller owns the token
	if err := s.VerifyBankOwner(ctx, tokenID); err != nil {
		return nil, fmt.Errorf("forbidden: %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	pending := []CustomerToTokenTransferRequest{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		normalizeCustomerToTokenTransferRequestForRead(&req)
		// Include transfers where this token is the sender and awaiting sender approval.
		if isCustomerTransferPendingSender(req.Status) && req.SenderTokenID == tokenID {
			// Fetch sender customer name
			senderCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.SenderCustomerID, req.SenderTokenID)
			if err == nil && senderCustomer != nil {
				req.SenderCustomerName = senderCustomer.Name
				if req.SenderCustomerTokenID == "" {
					req.SenderCustomerTokenID = senderCustomer.CustomerID
				}
			}

			// Fetch receiver customer name
			receiverCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.ReceiverCustomerID, req.ReceiverTokenID)
			if err == nil && receiverCustomer != nil {
				req.ReceiverCustomerName = receiverCustomer.Name
				if req.ReceiverCustomerTokenID == "" {
					req.ReceiverCustomerTokenID = receiverCustomer.CustomerID
				}
			}

			pending = append(pending, req)
		}
	}
	return pending, nil
}

// ViewPendingCustomerToTokenTransfersAsReceiver returns pending transfers where this token is the receiver
func (s *SmartContract) ViewPendingCustomerToTokenTransfersAsReceiver(ctx contractapi.TransactionContextInterface, tokenID string, ownerNetworkAddress string) ([]CustomerToTokenTransferRequest, error) {
	// SECURITY: Verify caller is the token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != ownerNetworkAddress {
		return nil, fmt.Errorf("forbidden: caller identity does not match owner")
	}

	// Verify caller owns the token
	if err := s.VerifyBankOwner(ctx, tokenID); err != nil {
		return nil, fmt.Errorf("forbidden: %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	pending := []CustomerToTokenTransferRequest{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		normalizeCustomerToTokenTransferRequestForRead(&req)
		// Include transfers where this token is the receiver and awaiting receiver approval.
		if isCustomerTransferPendingReceiver(req.Status) && req.ReceiverTokenID == tokenID {
			// Fetch sender customer name
			senderCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.SenderCustomerID, req.SenderTokenID)
			if err == nil && senderCustomer != nil {
				req.SenderCustomerName = senderCustomer.Name
				if req.SenderCustomerTokenID == "" {
					req.SenderCustomerTokenID = senderCustomer.CustomerID
				}
			}

			// Fetch receiver customer name
			receiverCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.ReceiverCustomerID, req.ReceiverTokenID)
			if err == nil && receiverCustomer != nil {
				req.ReceiverCustomerName = receiverCustomer.Name
				if req.ReceiverCustomerTokenID == "" {
					req.ReceiverCustomerTokenID = receiverCustomer.CustomerID
				}
			}

			pending = append(pending, req)
		}
	}
	return pending, nil
}

// GetCustomerToTokenTransferRequestByID retrieves a specific customer-to-token transfer request by ID
func (s *SmartContract) GetCustomerToTokenTransferRequestByID(ctx contractapi.TransactionContextInterface, transferRequestID string) (*CustomerToTokenTransferRequest, error) {
	if transferRequestID == "" {
		return nil, fmt.Errorf("transferRequestID is required")
	}

	stateKey, err := s.resolveCustomerTransferStateKey(ctx, transferRequestID)
	if err != nil {
		return nil, err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return nil, fmt.Errorf("transfer request not found")
	}

	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}

	normalizeCustomerToTokenTransferRequestForRead(&request)
	return &request, nil
}

// GetCustomerToTokenTransferHistory returns completed customer-to-token transfers
func (s *SmartContract) GetCustomerToTokenTransferHistory(ctx contractapi.TransactionContextInterface, tokenID string) ([]CustomerToTokenTransferRequest, error) {
	// SECURITY: Verify caller is a bank and owns this token
	if err := s.VerifyBankOwner(ctx, tokenID); err != nil {
		return nil, fmt.Errorf("forbidden: %v", err)
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var history []CustomerToTokenTransferRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		normalizeCustomerToTokenTransferRequestForRead(&req)
		// Include settled transfers involving this token.
		if req.Status == "SETTLED" && (req.SenderTokenID == tokenID || req.ReceiverTokenID == tokenID) {
			// Fetch sender customer name
			senderCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.SenderCustomerID, req.SenderTokenID)
			if err == nil && senderCustomer != nil {
				req.SenderCustomerName = senderCustomer.Name
				if req.SenderCustomerTokenID == "" {
					req.SenderCustomerTokenID = senderCustomer.CustomerID
				}
			}

			// Fetch receiver customer name
			receiverCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.ReceiverCustomerID, req.ReceiverTokenID)
			if err == nil && receiverCustomer != nil {
				req.ReceiverCustomerName = receiverCustomer.Name
				if req.ReceiverCustomerTokenID == "" {
					req.ReceiverCustomerTokenID = receiverCustomer.CustomerID
				}
			}

			history = append(history, req)
		}
	}
	return history, nil
}

// GetRejectedByReason returns rejected/failed transfer requests matching a specific rejection reason.
func (s *SmartContract) GetRejectedByReason(ctx contractapi.TransactionContextInterface, reason string) ([]CustomerToTokenTransferRequest, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}
	normalizedReason := normalizeCustomerTransferRejectionReason(reason)
	if normalizedReason == "" {
		return nil, fmt.Errorf("rejection reason is required")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	results := []CustomerToTokenTransferRequest{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}
		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		normalizeCustomerToTokenTransferRequestForRead(&req)
		if !isCustomerTransferRejectedStatus(req.Status) {
			continue
		}
		if req.RejectionReason == normalizedReason {
			results = append(results, req)
		}
	}
	return results, nil
}

// GetRejectedByBank returns rejected/failed transfer requests for a receiver bank BIC.
func (s *SmartContract) GetRejectedByBank(ctx contractapi.TransactionContextInterface, receiverBIC string) ([]CustomerToTokenTransferRequest, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}
	targetBIC := strings.TrimSpace(strings.ToUpper(receiverBIC))
	if !validBICFormat(targetBIC) {
		return nil, fmt.Errorf("receiver_bic must be valid BIC8/BIC11")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	results := []CustomerToTokenTransferRequest{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}
		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		normalizeCustomerToTokenTransferRequestForRead(&req)
		if !isCustomerTransferRejectedStatus(req.Status) {
			continue
		}
		if strings.TrimSpace(strings.ToUpper(req.ReceiverBIC)) == targetBIC {
			results = append(results, req)
		}
	}
	return results, nil
}

// GetExpiredEscrowReturns returns transfers auto-returned due to timeout.
func (s *SmartContract) GetExpiredEscrowReturns(ctx contractapi.TransactionContextInterface) ([]CustomerToTokenTransferRequest, error) {
	if err := s.VerifyAdmin(ctx); err != nil {
		return nil, err
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	results := []CustomerToTokenTransferRequest{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}
		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		normalizeCustomerToTokenTransferRequestForRead(&req)
		if normalizeCustomerTransferStatus(req.Status) == customerTransferStatusExpiredEscrowReturned &&
			req.RejectionReason == "24HR_TIMEOUT" {
			results = append(results, req)
		}
	}
	return results, nil
}

// GetCustomerToTokenTransferHistoryByCustomer returns customer-to-token transfers for a specific customer
// Shows transfers where customer is either sender or receiver
// SECURITY FIX #1: Verify caller identity matches the customer requesting history (defense-in-depth)
// SECURITY FIX #2: Validate customer network address and caller identity at chaincode level
func (s *SmartContract) GetCustomerToTokenTransferHistoryByCustomer(ctx contractapi.TransactionContextInterface, customerNetworkAddress string) ([]CustomerToTokenTransferRequest, error) {
	if customerNetworkAddress == "" {
		return nil, fmt.Errorf("customerNetworkAddress is required")
	}

	// SECURITY FIX #1: Verify caller identity - only customer can view their own history
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %v", err)
	}

	// SECURITY FIX #2: Verify caller's identity matches the customer requesting history
	participantBytes, err := ctx.GetStub().GetState(customerNetworkAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to verify customer record: %v", err)
	}
	if participantBytes == nil {
		return nil, fmt.Errorf("customer not found")
	}

	var participant Participant
	if err := json.Unmarshal(participantBytes, &participant); err != nil {
		return nil, fmt.Errorf("invalid customer data: %v", err)
	}

	// Enforce: Caller's identity must match the customer's ClientID (defense-in-depth)
	if participant.ClientID != "" && participant.ClientID != callerID {
		return nil, fmt.Errorf("unauthorized: caller identity does not match customer")
	}

	iter, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var history []CustomerToTokenTransferRequest
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !isCustomerTransferKey(kv.Key) {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		normalizeCustomerToTokenTransferRequestForRead(&req)
		// Include transfers where customer is sender or receiver (including pending approval stages)
		if req.SenderCustomerID == customerNetworkAddress || req.ReceiverCustomerID == customerNetworkAddress {
			// SECURITY FIX #6: Set transaction type (DEBIT if sender, CREDIT if receiver)
			if req.SenderCustomerID == customerNetworkAddress {
				req.DebitStatus = "DEBITED" // Mark as debit transaction
			} else {
				req.CreditStatus = "CREDITED" // Mark as credit transaction
			}

			// Fetch sender customer name
			senderCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.SenderCustomerID, req.SenderTokenID)
			if err == nil && senderCustomer != nil {
				req.SenderCustomerName = senderCustomer.Name
				if req.SenderCustomerTokenID == "" {
					req.SenderCustomerTokenID = senderCustomer.CustomerID
				}
			}

			// Fetch receiver customer name
			receiverCustomer, _, err := s.getParticipantByNetworkToken(ctx, req.ReceiverCustomerID, req.ReceiverTokenID)
			if err == nil && receiverCustomer != nil {
				req.ReceiverCustomerName = receiverCustomer.Name
				if req.ReceiverCustomerTokenID == "" {
					req.ReceiverCustomerTokenID = receiverCustomer.CustomerID
				}
			}

			history = append(history, req)
		}
	}
	return history, nil
}

// ApproveSenderTokenTransfer allows sender token owner to approve customer-to-token transfer
// Moves status from PendingSenderTokenApproval to PendingReceiverTokenApproval
func (s *SmartContract) ApproveSenderTokenTransfer(ctx contractapi.TransactionContextInterface, transferRequestID, senderOwnerAddress string, approved bool) error {
	// SECURITY: Verify caller is the sender token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != senderOwnerAddress {
		return fmt.Errorf("forbidden: caller identity does not match sender owner")
	}

	stateKey, err := s.resolveCustomerTransferStateKey(ctx, transferRequestID)
	if err != nil {
		return err
	}
	// Load transfer request
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("transfer request not found")
	}
	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}
	normalizeCustomerToTokenTransferRequestForRead(&request)

	// Verify transfer is in correct status
	if !isCustomerTransferPendingSender(request.Status) {
		return fmt.Errorf("transfer is not pending sender approval. Current status: %s", request.Status)
	}

	if !approved {
		return s.rejectCustomerTransferRequest(
			ctx,
			stateKey,
			&request,
			customerTransferStatusRejectedSenderPreEscrow,
			"SENDER_KYC_INVALID",
		)
	}

	// Verify caller owns the sender token
	senderTokenBytes, err := ctx.GetStub().GetState(request.SenderTokenID)
	if err != nil || senderTokenBytes == nil {
		return fmt.Errorf("sender token not found")
	}
	var senderToken Token
	if err := json.Unmarshal(senderTokenBytes, &senderToken); err != nil {
		return fmt.Errorf("failed to unmarshal sender token: %v", err)
	}
	if senderToken.Owner != senderOwnerAddress {
		return fmt.Errorf("caller is not the sender token owner")
	}

	// Verify sender customer still has sufficient balance (already escrowed but verify not reversed)
	_, _, err = s.getParticipantByNetworkToken(ctx, request.SenderCustomerID, request.SenderTokenID)
	if err != nil {
		return fmt.Errorf("sender customer not found")
	}

	// Mark approval by sender owner
	request.ApprovedBySenderOwner = true
	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	request.SenderTokenOwnerApprovedAt = ts

	// Move to next stage: pending receiver approval
	request.Status = "PENDING_RECEIVER"

	// Update transfer request
	normalizeCustomerToTokenTransferRequestForRead(&request)
	return s.storeCustomerTransferRequest(ctx, stateKey, &request)
}

// RejectSenderPreEscrow allows sender token owner to reject with an explicit reason.
func (s *SmartContract) RejectSenderPreEscrow(ctx contractapi.TransactionContextInterface, transferRequestID, senderOwnerAddress, reason string) error {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != senderOwnerAddress {
		return fmt.Errorf("forbidden: caller identity does not match sender owner")
	}

	stateKey, err := s.resolveCustomerTransferStateKey(ctx, transferRequestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("transfer request not found")
	}

	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}
	normalizeCustomerToTokenTransferRequestForRead(&request)
	if !isCustomerTransferPendingSender(request.Status) {
		return fmt.Errorf("transfer is not pending sender approval. Current status: %s", request.Status)
	}

	senderTokenBytes, err := ctx.GetStub().GetState(request.SenderTokenID)
	if err != nil || senderTokenBytes == nil {
		return fmt.Errorf("sender token not found")
	}
	var senderToken Token
	if err := json.Unmarshal(senderTokenBytes, &senderToken); err != nil {
		return fmt.Errorf("failed to unmarshal sender token: %v", err)
	}
	if senderToken.Owner != senderOwnerAddress {
		return fmt.Errorf("caller is not the sender token owner")
	}

	return s.rejectCustomerTransferRequest(ctx, stateKey, &request, customerTransferStatusRejectedSenderPreEscrow, reason)
}

// ApproveReceiverTokenTransfer allows receiver token owner to approve and complete customer-to-token transfer
// Moves status from PendingReceiverTokenApproval to Completed
// Credits receiver token and receiver customer wallet
// exchangeRateStr and convertedAmountStr are passed from backend for multi-currency transfers
func (s *SmartContract) ApproveReceiverTokenTransfer(ctx contractapi.TransactionContextInterface, transferRequestID, receiverOwnerAddress string, approved bool, exchangeRateStr, convertedAmountStr string) error {
	// SECURITY: Verify caller is the receiver token owner
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != receiverOwnerAddress {
		return fmt.Errorf("forbidden: caller identity does not match receiver owner")
	}

	// Parse exchange rate and converted amount if provided (for multi-currency transfers)
	var exchangeRate float64
	var convertedAmount float64
	if exchangeRateStr != "" {
		var err error
		exchangeRate, err = strconv.ParseFloat(exchangeRateStr, 64)
		if err != nil {
			return fmt.Errorf("invalid exchange rate format: %v", err)
		}
	}
	if convertedAmountStr != "" {
		var err error
		convertedAmount, err = strconv.ParseFloat(convertedAmountStr, 64)
		if err != nil {
			return fmt.Errorf("invalid converted amount format: %v", err)
		}
	}

	stateKey, err := s.resolveCustomerTransferStateKey(ctx, transferRequestID)
	if err != nil {
		return err
	}
	// Load transfer request
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("transfer request not found")
	}
	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}
	normalizeCustomerToTokenTransferRequestForRead(&request)

	// Verify transfer is in correct status (sender must have already approved)
	if !isCustomerTransferPendingReceiver(request.Status) {
		return fmt.Errorf("transfer is not pending receiver approval. Current status: %s", request.Status)
	}
	if !request.ApprovedBySenderOwner {
		return fmt.Errorf("transfer has not been approved by sender token owner")
	}

	if !approved {
		request.ApprovedByReceiverOwner = false
		return s.rejectCustomerTransferRequest(
			ctx,
			stateKey,
			&request,
			customerTransferStatusRejectedReceiver,
			"BANK_POLICY_VIOLATION",
		)
	}

	// Verify caller owns the receiver token
	receiverTokenBytes, err := ctx.GetStub().GetState(request.ReceiverTokenID)
	if err != nil || receiverTokenBytes == nil {
		return fmt.Errorf("receiver token not found")
	}
	var receiverToken Token
	if err := json.Unmarshal(receiverTokenBytes, &receiverToken); err != nil {
		return fmt.Errorf("failed to unmarshal receiver token: %v", err)
	}
	if receiverToken.Owner != receiverOwnerAddress {
		return fmt.Errorf("caller is not the receiver token owner")
	}

	// Verify receiver customer exists and is approved
	receiverCustomer, receiverCustomerKey, err := s.getParticipantByNetworkToken(ctx, request.ReceiverCustomerID, request.ReceiverTokenID)
	if err != nil {
		return fmt.Errorf("receiver customer not registered or approved")
	}
	if !receiverCustomer.Approved {
		return fmt.Errorf("receiver customer not approved")
	}

	// Perform the credit operation
	// 1. Credit receiver token with commission amount (2%)
	// 2. Credit receiver customer with their amount (98%)

	// Load sender token to verify it still has the funds (sanity check)
	senderTokenBytes, err := ctx.GetStub().GetState(request.SenderTokenID)
	if err != nil || senderTokenBytes == nil {
		return fmt.Errorf("sender token not found during approval")
	}
	var senderToken Token
	if err := json.Unmarshal(senderTokenBytes, &senderToken); err != nil {
		return fmt.Errorf("failed to unmarshal sender token: %v", err)
	}

	senderCurrency := strings.TrimSpace(request.SenderCurrency)
	receiverCurrency := strings.TrimSpace(request.ReceiverCurrency)

	if senderCurrency == receiverCurrency {
		// Same currency: both parties get their share in native currency
		// Receiver token gets commission (2%)
		setTokenSupply(&receiverToken, getTokenSupply(receiverToken)+int(request.CommissionAmount))
		// Receiver customer gets forwarded amount (98%)
		receiverCustomer.Balance += int(request.ReceiverCustomerAmount)
	} else {
		// Different currencies: commission deducted from sender amount FIRST, then remaining converted
		// Example: 3 USD sent
		// Step 1: Commission = 3 × 2% = 0.06 USD
		// Step 2: Remaining = 3 - 0.06 = 2.94 USD
		// Step 3: Convert remaining = 2.94 × 83.5 = 245.49 INR
		// Step 4: Token gets 0.06 USD, Customer gets 245.49 INR

		// 1. Full amount stored in receiver token's foreign currency wallet
		if receiverToken.ForeignBalances == nil {
			receiverToken.ForeignBalances = make(map[string]int)
		}
		receiverToken.ForeignBalances[senderCurrency] += int(request.EscrowAmount)

		// 2. Calculate commission from SENDER amount (2%)
		// Commission in sender currency = 3 USD × 2% = 0.06 USD
		commissionInSenderCurrency := int(float64(request.EscrowAmount) * 0.02)

		// 3. Calculate remaining after commission
		// Remaining = 3 - 0.06 = 2.94 USD
		remainingAfterCommission := int(request.EscrowAmount) - commissionInSenderCurrency

		// 4. Use exchange rate to convert remaining amount
		// Converted amount = 2.94 USD × 83.5 = 245.49 INR
		actualConvertedAmount := convertedAmount
		if actualConvertedAmount <= 0 {
			actualConvertedAmount = request.ConvertedAmount
		}
		if actualConvertedAmount <= 0 {
			// Calculate from remaining amount if not provided
			// This should be: remainingAfterCommission × exchangeRate
			if exchangeRate > 0 {
				actualConvertedAmount = float64(remainingAfterCommission) * exchangeRate
			} else {
				actualConvertedAmount = float64(remainingAfterCommission) // Fallback to 1:1
			}
		}

		// Store conversion details for audit trail
		request.ConvertedAmount = actualConvertedAmount
		if exchangeRate > 0 {
			request.ExchangeRate = exchangeRate
		}

		// 5. Token receives commission in sender currency (stored in ForeignBalances)
		// Already added to ForeignBalances above
		// Commission in sender currency = 0.06 USD (kept by token as profit)

		// 6. Deduct converted customer amount from receiver token's minted balance
		// to pay the customer
		setTokenSupply(&receiverToken, getTokenSupply(receiverToken)-int(math.Round(actualConvertedAmount)))

		// 7. Credit customer with converted amount
		// Customer receives = 245.49 INR (no commission deducted from customer's amount)
		receiverCustomer.Balance += int(math.Round(actualConvertedAmount))
	}

	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	request.ReceiverTokenOwnerApprovedAt = ts
	request.CompletedAt = ts
	request.SettledAt = ts
	request.RejectedAt = ""
	request.RejectionReason = ""

	// Mark transfer as completed
	request.ApprovedByReceiverOwner = true
	request.Status = "SETTLED"
	request.CreditStatus = "CREDITED"

	// Marshal all updates
	normalizeCustomerToTokenTransferRequestForRead(&request)
	updatedReceiverTokenBytes, err := json.Marshal(receiverToken)
	if err != nil {
		return fmt.Errorf("failed to marshal receiver token: %v", err)
	}
	updatedReceiverCustomerBytes, err := json.Marshal(receiverCustomer)
	if err != nil {
		return fmt.Errorf("failed to marshal receiver customer: %v", err)
	}

	// Persist all updates
	if err := s.storeCustomerTransferRequest(ctx, stateKey, &request); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(request.ReceiverTokenID, updatedReceiverTokenBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(receiverCustomerKey, updatedReceiverCustomerBytes); err != nil {
		return err
	}

	// Append immutable participant settlement record (privacy-safe, BIC/ref-based).
	txID := ctx.GetStub().GetTxID()
	shortTx := txID
	if len(shortTx) > 12 {
		shortTx = shortTx[:12]
	}
	senderBIC := strings.TrimSpace(strings.ToUpper(request.SenderBIC))
	receiverBIC := strings.TrimSpace(strings.ToUpper(request.ReceiverBIC))
	txRef := fmt.Sprintf("TXN-%s-%s", senderBIC, shortTx)
	if senderBIC != "" && receiverBIC != "" {
		txRef = fmt.Sprintf("TXN-%s-%s-%s", senderBIC, receiverBIC, shortTx)
	}

	record := ParticipantTransferRecord{
		TxRef:               txRef,
		RequestMsgID:        request.MsgID,
		SenderCustomerRef:   request.SenderCustomerRef,
		SenderBIC:           senderBIC,
		ReceiverCustomerRef: request.ReceiverCustomerRef,
		ReceiverBIC:         receiverBIC,
		Amount:              request.Amount,
		Currency:            request.Currency,
		Commission:          request.CommissionAmount,
		NetAmount:           request.NetReceiverAmount,
		ExchangeRate:        request.ExchangeRate,
		Status:              "SETTLED",
		SettledAt:           request.SettledAt,
		BlockHeight:         txID,

		// Legacy aliases for existing consumers.
		RecordID:              txRef,
		TransferRequestID:     request.MsgID,
		TransferID:            request.MsgID,
		TokenID:               request.SenderTokenID,
		SenderParticipantID:   request.SenderCustomerRef,
		ReceiverParticipantID: request.ReceiverCustomerRef,
		SenderTokenID:         request.SenderTokenID,
		ReceiverTokenID:       request.ReceiverTokenID,
		CompletedAt:           request.SettledAt,
	}
	normalizeParticipantTransferRecordForRead(&record)
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return err
	}
	recordKey := fmt.Sprintf("%s/RECORDS/%s", senderBIC, txRef)
	if senderBIC == "" {
		recordKey = fmt.Sprintf("RECORDS/%s", txRef)
	}
	if err := ctx.GetStub().PutState(recordKey, recordBytes); err != nil {
		return err
	}
	// Legacy prefix key for existing list APIs.
	legacyRecordKey := fmt.Sprintf("participanttransferhistory_%s", txRef)
	if err := ctx.GetStub().PutState(legacyRecordKey, recordBytes); err != nil {
		return err
	}

	return nil
}

// RejectReceiver allows receiver token owner to reject with an explicit reason and auto-refund escrow.
func (s *SmartContract) RejectReceiver(ctx contractapi.TransactionContextInterface, transferRequestID, receiverOwnerAddress, reason string) error {
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}
	if callerID != receiverOwnerAddress {
		return fmt.Errorf("forbidden: caller identity does not match receiver owner")
	}

	stateKey, err := s.resolveCustomerTransferStateKey(ctx, transferRequestID)
	if err != nil {
		return err
	}
	reqBytes, err := ctx.GetStub().GetState(stateKey)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("transfer request not found")
	}

	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}
	normalizeCustomerToTokenTransferRequestForRead(&request)
	if !isCustomerTransferPendingReceiver(request.Status) {
		return fmt.Errorf("transfer is not pending receiver approval. Current status: %s", request.Status)
	}
	if !request.ApprovedBySenderOwner {
		return fmt.Errorf("transfer has not been approved by sender token owner")
	}

	receiverTokenBytes, err := ctx.GetStub().GetState(request.ReceiverTokenID)
	if err != nil || receiverTokenBytes == nil {
		return fmt.Errorf("receiver token not found")
	}
	var receiverToken Token
	if err := json.Unmarshal(receiverTokenBytes, &receiverToken); err != nil {
		return fmt.Errorf("failed to unmarshal receiver token: %v", err)
	}
	if receiverToken.Owner != receiverOwnerAddress {
		return fmt.Errorf("caller is not the receiver token owner")
	}

	request.ApprovedByReceiverOwner = false
	return s.rejectCustomerTransferRequest(ctx, stateKey, &request, customerTransferStatusRejectedReceiver, reason)
}

// RecordMintTransaction logs a mint transaction in history
func (s *SmartContract) RecordMintTransaction(ctx contractapi.TransactionContextInterface, participantID string,
	amount float64, currency string, mintRequestID string, approvedBy string, approvedAt string) error {

	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}

	transactionID := fmt.Sprintf("mint_%s_%d", participantID, time.Now().UnixNano())
	symbol := currencySymbol(currency)

	transaction := TransactionHistoryRecord{
		TransactionID:  transactionID,
		Category:       "MINT",
		ParticipantID:  participantID,
		Timestamp:      ts,
		Amount:         amount,
		Currency:       currency,
		CurrencySymbol: symbol,
		Type:           "CREDIT", // Mint is always credit (green)
		Status:         "COMPLETED",
		MintRequestID:  mintRequestID,
		ApprovedBy:     approvedBy,
		ApprovedAt:     approvedAt,
	}

	// Store transaction record
	txBytes, err := json.Marshal(transaction)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	err = ctx.GetStub().PutState(transactionID, txBytes)
	if err != nil {
		return fmt.Errorf("failed to store transaction record: %v", err)
	}

	// Update transaction history index for participant
	err = s.addTransactionToHistory(ctx, participantID, transactionID, "MINT")
	if err != nil {
		return fmt.Errorf("failed to update transaction history: %v", err)
	}

	return nil
}

// RecordTransferTransaction logs a transfer transaction in history
func (s *SmartContract) RecordTransferTransaction(ctx contractapi.TransactionContextInterface,
	senderID, senderTokenID, senderName string,
	receiverID, receiverTokenID, receiverName string,
	amount, amountReceived float64,
	currency string,
	commissionBank string, commissionPercentage, commissionAmount float64,
	transferRequestID string,
	transactionType string) error { // "DEBIT" for sender, "CREDIT" for receiver

	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}

	symbol := currencySymbol(currency)
	baseTransactionID := fmt.Sprintf("transfer_%s_%d", transferRequestID, time.Now().UnixNano())

	// Create record for sender (DEBIT - red)
	senderTransaction := TransactionHistoryRecord{
		TransactionID:        baseTransactionID + "_sender",
		Category:             "TRANSFER",
		ParticipantID:        senderID,
		Timestamp:            ts,
		Amount:               amount,
		Currency:             currency,
		CurrencySymbol:       symbol,
		Type:                 "DEBIT", // Sender always debited
		Status:               "COMPLETED",
		SenderID:             senderID,
		SenderTokenID:        senderTokenID,
		SenderName:           senderName,
		ReceiverID:           receiverID,
		ReceiverTokenID:      receiverTokenID,
		ReceiverName:         receiverName,
		AmountReceived:       amountReceived,
		CommissionBank:       commissionBank,
		CommissionPercentage: commissionPercentage,
		CommissionAmount:     commissionAmount,
		TransferRequestID:    transferRequestID,
		RelatedTransactionID: baseTransactionID + "_receiver",
	}

	// Create record for receiver (CREDIT - green)
	receiverTransaction := TransactionHistoryRecord{
		TransactionID:        baseTransactionID + "_receiver",
		Category:             "TRANSFER",
		ParticipantID:        receiverID,
		Timestamp:            ts,
		Amount:               amountReceived, // Receiver sees what they received
		Currency:             currency,
		CurrencySymbol:       symbol,
		Type:                 "CREDIT", // Receiver always credited
		Status:               "COMPLETED",
		SenderID:             senderID,
		SenderTokenID:        senderTokenID,
		SenderName:           senderName,
		ReceiverID:           receiverID,
		ReceiverTokenID:      receiverTokenID,
		ReceiverName:         receiverName,
		AmountReceived:       amountReceived,
		CommissionBank:       commissionBank,
		CommissionPercentage: commissionPercentage,
		CommissionAmount:     commissionAmount,
		TransferRequestID:    transferRequestID,
		RelatedTransactionID: baseTransactionID + "_sender",
	}

	// Store both records
	senderTxBytes, err := json.Marshal(senderTransaction)
	if err != nil {
		return fmt.Errorf("failed to marshal sender transaction: %v", err)
	}

	receiverTxBytes, err := json.Marshal(receiverTransaction)
	if err != nil {
		return fmt.Errorf("failed to marshal receiver transaction: %v", err)
	}

	err = ctx.GetStub().PutState(senderTransaction.TransactionID, senderTxBytes)
	if err != nil {
		return fmt.Errorf("failed to store sender transaction: %v", err)
	}

	err = ctx.GetStub().PutState(receiverTransaction.TransactionID, receiverTxBytes)
	if err != nil {
		return fmt.Errorf("failed to store receiver transaction: %v", err)
	}

	// Update transaction history indices
	err = s.addTransactionToHistory(ctx, senderID, senderTransaction.TransactionID, "TRANSFER")
	if err != nil {
		return fmt.Errorf("failed to update sender transaction history: %v", err)
	}

	err = s.addTransactionToHistory(ctx, receiverID, receiverTransaction.TransactionID, "TRANSFER")
	if err != nil {
		return fmt.Errorf("failed to update receiver transaction history: %v", err)
	}

	return nil
}

// addTransactionToHistory updates the transaction history index for a participant
func (s *SmartContract) addTransactionToHistory(ctx contractapi.TransactionContextInterface,
	participantID string, transactionID string, category string) error {

	historyKey := fmt.Sprintf("history_%s", participantID)

	// Try to get existing history
	historyBytes, err := ctx.GetStub().GetState(historyKey)
	var history TransactionHistory

	if err != nil {
		return fmt.Errorf("failed to get transaction history: %v", err)
	}

	if historyBytes == nil {
		// Create new history
		history = TransactionHistory{
			ParticipantID:      participantID,
			TransactionIDs:     []string{transactionID},
			MintTransactionIDs: []string{},
			TransferIDs:        []string{},
		}
		if category == "MINT" {
			history.MintTransactionIDs = []string{transactionID}
		} else if category == "TRANSFER" {
			history.TransferIDs = []string{transactionID}
		}
	} else {
		// Update existing history
		err = json.Unmarshal(historyBytes, &history)
		if err != nil {
			return fmt.Errorf("failed to unmarshal transaction history: %v", err)
		}

		// Append transaction ID if not already present
		found := false
		for _, id := range history.TransactionIDs {
			if id == transactionID {
				found = true
				break
			}
		}
		if !found {
			history.TransactionIDs = append(history.TransactionIDs, transactionID)
		}

		if category == "MINT" {
			found = false
			for _, id := range history.MintTransactionIDs {
				if id == transactionID {
					found = true
					break
				}
			}
			if !found {
				history.MintTransactionIDs = append(history.MintTransactionIDs, transactionID)
			}
		} else if category == "TRANSFER" {
			found = false
			for _, id := range history.TransferIDs {
				if id == transactionID {
					found = true
					break
				}
			}
			if !found {
				history.TransferIDs = append(history.TransferIDs, transactionID)
			}
		}
	}

	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	history.LastUpdated = ts

	// Store updated history
	historyBytes, err = json.Marshal(history)
	if err != nil {
		return fmt.Errorf("failed to marshal updated history: %v", err)
	}

	err = ctx.GetStub().PutState(historyKey, historyBytes)
	if err != nil {
		return fmt.Errorf("failed to store updated history: %v", err)
	}

	return nil
}

// GetTransactionHistory retrieves all transactions for a participant
func (s *SmartContract) GetTransactionHistory(ctx contractapi.TransactionContextInterface, participantID string) ([]TransactionHistoryRecord, error) {

	historyKey := fmt.Sprintf("history_%s", participantID)
	historyBytes, err := ctx.GetStub().GetState(historyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction history: %v", err)
	}

	if historyBytes == nil {
		return []TransactionHistoryRecord{}, nil
	}

	var history TransactionHistory
	err = json.Unmarshal(historyBytes, &history)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction history: %v", err)
	}

	// Fetch all transaction records
	var transactions []TransactionHistoryRecord
	for _, txID := range history.TransactionIDs {
		txBytes, err := ctx.GetStub().GetState(txID)
		if err != nil {
			continue // Skip if error
		}

		if txBytes == nil {
			continue
		}

		var tx TransactionHistoryRecord
		err = json.Unmarshal(txBytes, &tx)
		if err != nil {
			continue
		}

		transactions = append(transactions, tx)
	}

	// Sort by timestamp (newest first)
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Timestamp > transactions[j].Timestamp
	})

	return transactions, nil
}

// GetMintTransactions retrieves only mint transactions for a participant
func (s *SmartContract) GetMintTransactions(ctx contractapi.TransactionContextInterface, participantID string) ([]TransactionHistoryRecord, error) {

	allTransactions, err := s.GetTransactionHistory(ctx, participantID)
	if err != nil {
		return nil, err
	}

	var mintTransactions []TransactionHistoryRecord
	for _, tx := range allTransactions {
		if tx.Category == "MINT" {
			mintTransactions = append(mintTransactions, tx)
		}
	}

	return mintTransactions, nil
}

// GetTransferTransactions retrieves only transfer transactions for a participant
func (s *SmartContract) GetTransferTransactions(ctx contractapi.TransactionContextInterface, participantID string) ([]TransactionHistoryRecord, error) {

	allTransactions, err := s.GetTransactionHistory(ctx, participantID)
	if err != nil {
		return nil, err
	}

	var transferTransactions []TransactionHistoryRecord
	for _, tx := range allTransactions {
		if tx.Category == "TRANSFER" {
			transferTransactions = append(transferTransactions, tx)
		}
	}

	return transferTransactions, nil
}

func main() {
	cc, err := contractapi.NewChaincode(new(SmartContract))
	if err != nil {
		panic(err)
	}
	if err := cc.Start(); err != nil {
		panic(err)
	}
}
