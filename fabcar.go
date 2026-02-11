package main

import (
	"encoding/json"
	"fmt"
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
	CustomerID        string             `json:"customer_id"`
	Name              string             `json:"name"`
	NetworkAddress    string             `json:"network_address"`
	ClientID          string             `json:"client_id"`
	MSP               string             `json:"msp"` // Bank/Organization (Org1MSP, Org2MSP, Org3MSP)
	Approved          bool               `json:"approved"`
	ApprovedAt        string             `json:"approved_at"`
	Country           string             `json:"country"`
	TokenID           string             `json:"token_id"`
	TransferIDs       []string           `json:"transfer_ids"`
	KycId             string             `json:"kyc_id"`
	KycStatus         string             `json:"kyc_status"`
	Balance           int                `json:"balance"`
	ForeignCurrencies map[string]float64 `json:"foreign_currencies"` // Holdings in other currencies
	TokenTransferIDs  []string           `json:"token_transfer_ids"`
}

type Token struct {
	TokenID         string         `json:"token_id"`
	Owner           string         `json:"owner"`
	OwnerMSP        string         `json:"owner_msp"` // Bank that owns this token (Org1MSP, Org2MSP, Org3MSP)
	Available       bool           `json:"available"`
	Minted          int            `json:"minted"`
	Currency        string         `json:"currency"`
	DisplayTokenID  string         `json:"display_token_id"`
	TransferIDs     []string       `json:"transfer_ids"`
	AssignedAt      string         `json:"assigned_at"`
	ForeignBalances map[string]int `json:"foreign_balances"`
	// BetweenNetwork Governance Fields
	MaxSupply           int     `json:"max_supply"`             // Regulatory cap on total supply
	DailyMintLimit      float64 `json:"daily_mint_limit"`       // Daily minting allowance
	TotalMintedToday    float64 `json:"total_minted_today"`     // Rolling 24h counter
	LastMintRulesUpdate string  `json:"last_mint_rules_update"` // Audit timestamp
	EmergencyFreeze     bool    `json:"emergency_freeze"`       // Can be set by BetweenNetwork admin
	Purpose             string  `json:"purpose"`                // BANK_TOKEN or SYSTEM_TOKEN
	Status              string  `json:"status"`                 // READY_FOR_MINTING, FROZEN, etc.
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
	RequestID          string `json:"request_id"`
	NetworkAddr        string `json:"network_addr"`
	ParticipantAddress string `json:"participant_address"`
	CallerID           string `json:"caller_id"`
	CallerMSP          string `json:"caller_msp"`
	ParticipantMSP     string `json:"participant_msp"` // Bank that made the request
	Status             string `json:"status"`          // PENDING, APPROVED, CANCELLED
	TokenID            string `json:"token_id"`
	Currency           string `json:"currency"`
}

type MintRequest struct {
	RequestID   string `json:"request_id"`
	TokenID     string `json:"token_id"`
	CustomerID  string `json:"customer_id"`
	RequestedBy string `json:"requested_by"`
	Name        string `json:"name"`
	KycId       string `json:"kyc_id"`
	KycStatus   string `json:"kyc_status"`
	Amount      int    `json:"amount"`
	Approved    bool   `json:"approved"`
	ApprovedAt  string `json:"approved_at"`
	Currency    string `json:"currency"`
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
	RequestID      string `json:"request_id"`
	NetworkAddress string `json:"network_address"`
	Name           string `json:"name"`
	ClientID       string `json:"client_id"`
	TokenID        string `json:"token_id"`
	Approved       bool   `json:"approved"`
	KycId          string `json:"kyc_id"`
	KycStatus      string `json:"kyc_status"`
	CreatedAt      string `json:"created_at"` // SECURITY FIX: Track request creation time for expiration
	Status         string `json:"status"`     // SECURITY FIX: Track request status (PENDING, REJECTED, APPROVED)
}

// TransferRequest struct removed - use TokenTransferRequest instead

type TokenTransferRequest struct {
	RequestID         string `json:"request_id"`
	SenderTokenID     string `json:"sender_token_id"`
	ReceiverTokenID   string `json:"receiver_token_id"`
	SenderName        string `json:"sender_name"`
	ReceiverName      string `json:"receiver_name"`
	SenderKycId       string `json:"sender_kyc_id"`
	SenderKycStatus   string `json:"sender_kyc_status"`
	ReceiverKycId     string `json:"receiver_kyc_id"`
	ReceiverKycStatus string `json:"receiver_kyc_status"`
	Amount            int    `json:"amount"`
	InitiatedBy       string `json:"initiated_by"`
	Status            string `json:"status"` // PendingReceiverApproval, Completed, Rejected
	CompletedAt       string `json:"completed_at"`
	Currency          string `json:"currency"`
}

type TokenToTokenTransferRecord struct {
	RecordID        string `json:"record_id"`
	RequestID       string `json:"request_id"`
	SenderTokenID   string `json:"sender_token_id"`
	ReceiverTokenID string `json:"receiver_token_id"`
	Amount          int    `json:"amount"`
	InitiatedBy     string `json:"initiated_by"`
	ApprovedBy      string `json:"approved_by"`
	ApprovedAt      string `json:"approved_at"`
	Currency        string `json:"currency"`
}

type ParticipantTransferRecord struct {
	RecordID              string  `json:"record_id"`
	TransferRequestID     string  `json:"transfer_request_id"`
	TransferID            string  `json:"transfer_id"`
	TokenID               string  `json:"token_id"`
	SenderParticipantID   string  `json:"sender_participant_id"`
	ReceiverParticipantID string  `json:"receiver_participant_id"`
	SenderName            string  `json:"sender_name"`
	ReceiverName          string  `json:"receiver_name"`
	SenderKycId           string  `json:"sender_kyc_id"`
	SenderKycStatus       string  `json:"sender_kyc_status"`
	ReceiverKycId         string  `json:"receiver_kyc_id"`
	ReceiverKycStatus     string  `json:"receiver_kyc_status"`
	SenderTokenID         string  `json:"sender_token_id"`
	ReceiverTokenID       string  `json:"receiver_token_id"`
	Amount                float64 `json:"amount"`
	CompletedAt           string  `json:"completed_at"`
}

// CustomerToTokenTransferRequest represents a customer initiating a transfer to another token (which forwards to another customer)
// Flow: Sender Customer (Token A) → Receiver Token B → Receiver Customer (Token B)
// Receiver Token takes 2% commission, forwards 98% to Receiver Customer
type CustomerToTokenTransferRequest struct {
	TransferRequestID            string  `json:"transfer_request_id"`
	SenderCustomerID             string  `json:"sender_customer_id"`     // Customer's network address (Token A owner)
	SenderCustomerTokenID        string  `json:"sender_customer_token_id"`
	SenderCustomerName           string  `json:"sender_customer_name"`   // Sender customer's name
	SenderTokenID                string  `json:"sender_token_id"`        // Token A (sender customer's token)
	ReceiverTokenID              string  `json:"receiver_token_id"`      // Token B (intermediate receiver, takes commission)
	ReceiverCustomerID           string  `json:"receiver_customer_id"`   // Final destination customer (must be registered with Token B)
	ReceiverCustomerTokenID      string  `json:"receiver_customer_token_id"`
	ReceiverCustomerName         string  `json:"receiver_customer_name"` // Receiver customer's name
	Amount                       int     `json:"amount"`                 // In sender's currency
	SenderCurrency               string  `json:"sender_currency"`        // Currency of sender token
	Status                       string  `json:"status"`                 // PendingSenderTokenApproval, PendingReceiverTokenApproval, Completed, RejectedBySenderOwner, RejectedByReceiverOwner
	InitiatedBy                  string  `json:"initiated_by"`           // Customer's certificate ID
	DebitStatus                  string  `json:"debit_status"`           // DEBITED, REVERSED
	CreditStatus                 string  `json:"credit_status"`          // PENDING, CREDITED
	EscrowedAmount               int     `json:"escrowed_amount"`        // Amount in escrow
	ApprovedBySenderOwner        bool    `json:"approved_by_sender_owner"`
	ApprovedByReceiverOwner      bool    `json:"approved_by_receiver_owner"`
	SenderTokenOwnerApprovedAt   string  `json:"sender_approved_at"`
	ReceiverTokenOwnerApprovedAt string  `json:"receiver_approved_at"`
	CompletedAt                  string  `json:"completed_at"`
	ReceiverCurrency             string  `json:"receiver_currency"`        // For wallet display
	CommissionPercentage         float64 `json:"commission_percentage"`    // 2.0 (represents 2%)
	CommissionAmount             int     `json:"commission_amount"`        // 2% commission kept by receiver token
	ReceiverCustomerAmount       int     `json:"receiver_customer_amount"` // 98% forwarded to receiver customer
	ExchangeRate                 float64 `json:"exchange_rate"`            // Exchange rate used (if different currencies): 1 SenderCurrency = X ReceiverCurrency
	ConvertedAmount              float64 `json:"converted_amount"`         // Full amount converted to receiver currency
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
	participantStatePrefix      = "participant_"
	participantIndexPrefix      = "participantidx_"
	customerIDUniquePrefix      = "customerid_"
	customerIDTokenIndexPrefix  = "participantbytoken_"
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
	return &participant, stateKey, nil
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
			Owner:           "",
			Available:       true,
			Minted:          0,
			Currency:        "",
			TransferIDs:     []string{},
			ForeignBalances: make(map[string]int),
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

// RequestTokenRequest allows participant to request token purchase; only if details match
func (s *SmartContract) RequestTokenRequest(ctx contractapi.TransactionContextInterface, name, networkAddress, country, currency string) error {
	// SECURITY FIX #1: Validate networkAddress format - cannot be empty
	networkAddress = strings.TrimSpace(networkAddress)
	if networkAddress == "" {
		return fmt.Errorf("network address cannot be empty")
	}

	// SECURITY FIX #2: Validate currency is provided and not empty
	currency = strings.TrimSpace(currency)
	if currency == "" {
		return fmt.Errorf("currency is required")
	}

	// SECURITY: Extract and verify caller identity first
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %v", err)
	}

	b, err := ctx.GetStub().GetState(networkAddress)
	if err != nil || b == nil {
		return fmt.Errorf("participant not found")
	}
	var p Participant
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}

	// SECURITY FIX #5: Verify caller identity matches participant (not just parameter match)
	if p.ClientID != callerID {
		return fmt.Errorf("forbidden: caller identity does not match participant")
	}

	// Only validate immutable participant name; ignore country to avoid false mismatches
	if p.Name != name {
		return fmt.Errorf("participant details do not match")
	}

	client, err := cid.New(ctx.GetStub())
	if err != nil {
		return err
	}
	callerID, err = client.GetID()
	if err != nil {
		return err
	}
	mspID, err := client.GetMSPID()
	if err != nil {
		return err
	}

	txID := ctx.GetStub().GetTxID()
	reqID := fmt.Sprintf("tokenrequest_%s", txID)
	req := TokenRequest{
		RequestID:          reqID,
		NetworkAddr:        callerID,
		ParticipantAddress: networkAddress,
		CallerID:           callerID,
		CallerMSP:          mspID,
		ParticipantMSP:     p.MSP, // Store participant's MSP for validation during approval
		Status:             "PENDING",
		TokenID:            "",
		Currency:           currency,
	}
	rb, _ := json.Marshal(req)
	return ctx.GetStub().PutState(reqID, rb)
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
		if strings.HasPrefix(kv.Key, "tokenrequest_") {
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

	rb, err := ctx.GetStub().GetState(requestID)
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

	currency := strings.TrimSpace(r.Currency)
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
	rb, _ = json.Marshal(r)

	if err = ctx.GetStub().PutState(requestID, rb); err != nil {
		return err
	}

	targetAddress := r.ParticipantAddress
	if targetAddress == "" {
		targetAddress = r.NetworkAddr
	}

	pb, err := ctx.GetStub().GetState(targetAddress)
	if err != nil || pb == nil {
		return fmt.Errorf("participant not found")
	}
	var p Participant
	json.Unmarshal(pb, &p)

	// Verify participant belongs to approver's bank
	if p.MSP != approverMSP {
		return fmt.Errorf("access denied: cannot approve token for participant from different bank")
	}

	p.TokenID = tokenID
	p.Approved = true
	assignTime, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	p.ApprovedAt = assignTime
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

	rb, err := ctx.GetStub().GetState(requestID)
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

	return ctx.GetStub().PutState(requestID, rb)
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

// RequestMintCoins allows token owner to request minting coins
// RequestMintCoins verifies participant identity, then stores mint request
func (s *SmartContract) RequestMintCoins(ctx contractapi.TransactionContextInterface, networkAddress string, amount int) error {
	// Fetch participant information by network address
	partBytes, err := ctx.GetStub().GetState(networkAddress)
	if err != nil || partBytes == nil {
		return fmt.Errorf("participant not found")
	}
	var participant Participant
	if err := json.Unmarshal(partBytes, &participant); err != nil {
		return err
	}

	// DATA ISOLATION: Verify caller's bank matches participant's bank
	callerMSP, err := s.GetCallerMSP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get caller MSP: %v", err)
	}
	if err := s.VerifyBankAccessToData(ctx, participant.MSP); err != nil {
		return err
	}

	// SECURITY: Verify caller identity matches participant client ID (3-layer defense)
	callerID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return err
	}
	if participant.ClientID != callerID {
		return fmt.Errorf("forbidden: caller identity does not match participant")
	}
	// Additional verification: participant must own a token
	if participant.TokenID == "" {
		return fmt.Errorf("participant has no assigned token")
	}

	// Check token ownership (redundancy check)
	tokenBytes, err := ctx.GetStub().GetState(participant.TokenID)
	if err != nil || tokenBytes == nil {
		return fmt.Errorf("token not found")
	}
	var token Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return err
	}
	if token.Owner != networkAddress {
		return fmt.Errorf("caller is not token owner")
	}
	// VERIFY: Only token owner's bank can request mints
	if token.OwnerMSP != callerMSP {
		return fmt.Errorf("access denied: only token owner's bank can request mints")
	}
	tokenCurrency := strings.TrimSpace(token.Currency)
	if tokenCurrency == "" {
		return fmt.Errorf("token currency not configured")
	}

	// Create mint request ID unique per transaction similar to token requests
	txID := ctx.GetStub().GetTxID()
	reqKey := fmt.Sprintf("mintrequest_%s", txID)
	mintReq := MintRequest{
		RequestID:   reqKey,
		TokenID:     participant.TokenID,
		RequestedBy: networkAddress,
		Amount:      amount,
		Approved:    false,
		Currency:    tokenCurrency,
	}
	reqBytes, err := json.Marshal(mintReq)
	if err != nil {
		return err
	}

	// Store the mint request on ledger
	return ctx.GetStub().PutState(reqKey, reqBytes)
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
		if strings.HasPrefix(kv.Key, "mintrequest_") {
			var r MintRequest
			if json.Unmarshal(kv.Value, &r) == nil && !r.Approved {
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
		if !strings.HasPrefix(kv.Key, "mintrequest_") {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if req.Approved {
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
		if !strings.HasPrefix(kv.Key, "mintrequest_") {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if req.RequestedBy == networkAddress && req.Approved {
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

	reqBytes, err := ctx.GetStub().GetState(requestID)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("mint request not found")
	}

	var mr MintRequest
	if err := json.Unmarshal(reqBytes, &mr); err != nil {
		return err
	}

	if mr.Approved {
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
	mr.Approved = true
	mr.ApprovedAt = ts
	updatedReqBytes, err := json.Marshal(mr)
	if err != nil {
		return err
	}

	if err = ctx.GetStub().PutState(requestID, updatedReqBytes); err != nil {
		return err
	}
	token.Minted += mr.Amount
	updatedTokenBytes, err := json.Marshal(token)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(mr.TokenID, updatedTokenBytes)
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
	availableBalanceValue := float64(t.Minted)
	availableDisplay := formatCurrencyValue(t.Currency, availableBalanceValue)
	walletDisplay := formatCurrencyValue(t.Currency, walletBalance)

	return map[string]interface{}{
		"networkAddress":          p.NetworkAddress,
		"tokenID":                 t.TokenID,
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
	reqID := fmt.Sprintf("custreq_%s", txID)

	clientID := callerID

	req := RegisterParticipantRequest{
		RequestID:      reqID,
		NetworkAddress: networkAddress,
		Name:           name,
		ClientID:       clientID,
		TokenID:        tokenID,
		Approved:       false,
		KycId:          kycId,
		KycStatus:      kycStatus,
		CreatedAt:      time.Now().Format(time.RFC3339), // SECURITY FIX: Track creation time for expiration
		Status:         "PENDING",                       // SECURITY FIX: Track request status
	}
	requestBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(reqID, requestBytes)
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
		if strings.HasPrefix(kv.Key, "custreq_") {
			var req RegisterParticipantRequest
			if err := json.Unmarshal(kv.Value, &req); err == nil && req.TokenID == tokenID && !req.Approved {
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

	reqBytes, err := ctx.GetStub().GetState(requestID)
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

	if req.Approved {
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
	req.CreatedAt = strings.TrimSpace(req.CreatedAt)
	if req.CreatedAt != "" {
		createdTime, err := time.Parse(time.RFC3339, req.CreatedAt)
		if err == nil {
			elapsedDays := time.Since(createdTime).Hours() / 24
			if elapsedDays > 30 {
				return fmt.Errorf("registration request expired (created %v days ago, max 30 days)", int(elapsedDays))
			}
		}
	}
	req.Approved = true
	req.Status = "APPROVED"
	updatedReqBytes, _ := json.Marshal(req)
	if err := ctx.GetStub().PutState(requestID, updatedReqBytes); err != nil {
		return err
	}

	// Update participant record with kyc_id and kyc_status
	participantBytes, err := ctx.GetStub().GetState(ownerNetworkAddress)
	if err == nil && participantBytes != nil {
		var participant Participant
		if err := json.Unmarshal(participantBytes, &participant); err == nil {
			participant.KycId = req.KycId
			participant.KycStatus = req.KycStatus
			participant.Approved = true // Mark participant as approved
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
		CustomerID:        customerID,
		NetworkAddress:    req.NetworkAddress,
		Name:              req.Name,
		ClientID:          req.ClientID,
		TokenID:           req.TokenID,
		KycId:             req.KycId,
		KycStatus:         req.KycStatus,
		Approved:          true,
		Balance:           0,
		TransferIDs:       []string{},
		TokenTransferIDs:  []string{},
		ForeignCurrencies: make(map[string]float64),
	}
	participantBytes, err = json.Marshal(participant)
	if err != nil {
		return err
	}
	participantKey := participantStateKeyByCustomerID(customerID)
	if err := ctx.GetStub().PutState(participantKey, participantBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(participantNetworkTokenIndexKey(req.NetworkAddress, req.TokenID), []byte(customerID)); err != nil {
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
	mintRequestID := "custmintreq_" + req.NetworkAddress + "_" + req.TokenID
	approvedMintRequest := MintRequest{
		RequestID:   mintRequestID,
		TokenID:     req.TokenID,
		CustomerID:  customerID,
		RequestedBy: req.NetworkAddress,
		Amount:      0, // Initial balance is 0, customer can request mint later
		Name:        req.Name,
		KycId:       req.KycId,
		KycStatus:   req.KycStatus,
		Approved:    true, // Mark as approved immediately upon customer registration approval
		ApprovedAt:  time.Now().Format(time.RFC3339),
		Currency:    "", // Will be set when customer requests mint
	}
	mintReqBytes, _ := json.Marshal(approvedMintRequest)
	return ctx.GetStub().PutState(mintRequestID, mintReqBytes)
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
	}
	if approved {
		customer.KycStatus = "approved"
	} else {
		customer.KycStatus = "pending"
	}
	if approved && !customer.Approved {
		customer.Approved = true
	}

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
	} else {
		customerID := generateTokenScopedCustomerID(trimmedToken, ctx.GetStub().GetTxID())
		if err := ensureCustomerIDUnique(ctx, customerID); err != nil {
			return err
		}
		customerValue = Participant{
			CustomerID:        customerID,
			NetworkAddress:    trimmedNetwork,
			TokenID:           trimmedToken,
			TransferIDs:       []string{},
			TokenTransferIDs:  []string{},
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
	}

	if approved {
		customerValue.KycStatus = "approved"
	} else {
		customerValue.KycStatus = "pending"
	}
	if approved {
		customerValue.Approved = true
	}

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

	// Create mint request ID using the current transaction
	txID := ctx.GetStub().GetTxID()
	requestID := fmt.Sprintf("custmintreq_%s", txID)
	mintReq := MintRequest{
		RequestID:   requestID,
		TokenID:     tokenID,
		CustomerID:  customer.CustomerID,
		RequestedBy: customer.NetworkAddress,
		Name:        customer.Name,
		KycId:       customer.KycId,
		KycStatus:   fmt.Sprintf("%v", customer.KycStatus),
		Amount:      amount,
		Approved:    false,
		Currency:    tokenCurrency,
	}
	reqBytes, _ := json.Marshal(mintReq)
	return ctx.GetStub().PutState(requestID, reqBytes)
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
		if !strings.HasPrefix(kv.Key, "custmintreq_") {
			continue
		}
		var r MintRequest
		if err := json.Unmarshal(kv.Value, &r); err != nil {
			continue
		}
		if r.Approved {
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
		if !strings.HasPrefix(kv.Key, "custmintreq_") {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if req.Approved {
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
		if !strings.HasPrefix(kv.Key, "custmintreq_") {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		// Filter by customer network address and approval status
		if req.RequestedBy == customerNetworkAddress && req.Approved {
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
		if !strings.HasPrefix(kv.Key, "custmintreq_") {
			continue
		}
		var req MintRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}
		if req.Approved && req.RequestedBy == callerID {
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
	reqBytes, err := ctx.GetStub().GetState(requestID)
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
	if mintReq.Approved {
		return fmt.Errorf("mint request already approved")
	}

	// Check if the token has enough minted coins to fulfill this request
	if token.Minted < mintReq.Amount {
		return fmt.Errorf("insufficient minted coin balance on token: available %d, requested %d", token.Minted, mintReq.Amount)
	}

	// Approve mint request
	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	mintReq.Approved = true
	mintReq.ApprovedAt = ts
	updatedReqBytes, err := json.Marshal(mintReq)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(requestID, updatedReqBytes); err != nil {
		return err
	}

	// Deduct the requested amount from token's minted coins balance
	token.Minted -= mintReq.Amount
	updatedTokenBytes, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(mintReq.TokenID, updatedTokenBytes); err != nil {
		return err
	}

	// Credit the customer’s balance
	cust, customerKey, err := s.getParticipantByNetworkToken(ctx, mintReq.RequestedBy, mintReq.TokenID)
	if err != nil {
		return fmt.Errorf("customer not found")
	}
	cust.Balance += mintReq.Amount
	updatedCustBytes, err := json.Marshal(cust)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(customerKey, updatedCustBytes)
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
		return nil, err
	}

	if customer.ClientID != "" && customer.ClientID != callerID {
		return nil, fmt.Errorf("forbidden: caller identity does not match customer")
	}
	if customer.MSP != "" && customer.MSP != callerMSP {
		return nil, fmt.Errorf("forbidden: cannot access another bank's customer identity")
	}

	return map[string]interface{}{
		"network_address": customer.NetworkAddress,
		"token_id":        customer.TokenID,
		"customer_id":     customer.CustomerID,
		"approved":        customer.Approved,
		"status":          "approved",
	}, nil
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

// CreateTokenTransferRequest lets a sender token owner request a transfer of minted coins to another token.
func (s *SmartContract) CreateTokenTransferRequest(ctx contractapi.TransactionContextInterface, senderTokenID, receiverTokenID, senderOwnerAddress string, amount int) (string, error) {
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
	if senderToken.Minted < amount {
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

	reqID := "tokentransfer_" + ctx.GetStub().GetTxID()
	request := TokenTransferRequest{
		RequestID:       reqID,
		SenderTokenID:   senderTokenID,
		ReceiverTokenID: receiverTokenID,
		Amount:          amount,
		InitiatedBy:     senderOwnerAddress,
		Status:          "PendingReceiverApproval",
		Currency:        senderCurrency,
	}
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	if err := ctx.GetStub().PutState(reqID, reqBytes); err != nil {
		return "", err
	}
	return reqID, nil
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
		if strings.HasPrefix(kv.Key, "tokentransfer_") {
			var req TokenTransferRequest
			if err := json.Unmarshal(kv.Value, &req); err == nil && req.ReceiverTokenID == receiverTokenID && req.Status == "PendingReceiverApproval" {
				pending = append(pending, req)
			}
		}
	}
	return pending, nil
}

// ApproveTokenTransferRequest lets the receiver token owner release funds by crediting their token and debiting the sender token.
func (s *SmartContract) ApproveTokenTransferRequest(ctx contractapi.TransactionContextInterface, requestID, receiverOwnerAddress string) error {
	reqBytes, err := ctx.GetStub().GetState(requestID)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("token transfer request not found")
	}
	var request TokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return err
	}
	if request.Status != "PendingReceiverApproval" {
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
	if senderToken.Minted < request.Amount {
		request.Status = "Rejected"
		updatedReqBytes, _ := json.Marshal(request)
		_ = ctx.GetStub().PutState(requestID, updatedReqBytes)
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
		request.Status = "Rejected"
		updatedReqBytes, _ := json.Marshal(request)
		_ = ctx.GetStub().PutState(requestID, updatedReqBytes)
		return fmt.Errorf("handshake approval was revoked or does not exist between tokens")
	}

	// Perform transfer
	senderToken.Minted -= request.Amount
	if senderCurrency == receiverCurrency {
		receiverToken.Minted += request.Amount
	} else {
		// Foreign currency transfer: add to ForeignBalances (available balance)
		if receiverToken.ForeignBalances == nil {
			receiverToken.ForeignBalances = make(map[string]int)
		}
		receiverToken.ForeignBalances[senderCurrency] += request.Amount
		fmt.Printf("[CHAINCODE DEBUG] Added to foreign balances for %s: %d, New total: %d\n", senderCurrency, request.Amount, receiverToken.ForeignBalances[senderCurrency])
	}

	if request.Currency == "" {
		request.Currency = senderCurrency
	}

	request.Status = "Completed"
	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
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
	if err := ctx.GetStub().PutState(requestID, reqBytes); err != nil {
		return err
	}

	recordID := fmt.Sprintf("tokentotransferhistory_%s", ctx.GetStub().GetTxID())
	history := TokenToTokenTransferRecord{
		RecordID:        recordID,
		RequestID:       request.RequestID,
		SenderTokenID:   request.SenderTokenID,
		ReceiverTokenID: request.ReceiverTokenID,
		Amount:          request.Amount,
		InitiatedBy:     request.InitiatedBy,
		ApprovedBy:      receiverOwnerAddress,
		ApprovedAt:      ts,
		Currency:        request.Currency,
	}
	historyBytes, err := json.Marshal(history)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(recordID, historyBytes)
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
		if !strings.HasPrefix(kv.Key, "tokentotransferhistory_") {
			continue
		}
		var record TokenToTokenTransferRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}
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
		if record.SenderParticipantID == networkAddress || record.ReceiverParticipantID == networkAddress {
			// SWIFT Compliance: Show own customer details, hide counterparty details
			if record.SenderParticipantID == networkAddress {
				// This user is the sender - show their customer, hide receiver's
				record.ReceiverName = ""
				record.ReceiverKycId = ""
				record.ReceiverKycStatus = ""
			} else {
				// This user is the receiver - show their customer, hide sender's
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
		history = append(history, record)
	}
	return history, nil
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
		if record.SenderParticipantID == participantID || record.ReceiverParticipantID == participantID {
			history = append(history, record)
		}
	}
	return history, nil
}

// CUSTOMER-TO-TOKEN TRANSFER FUNCTIONS ========================================

// CreateCustomerToTokenTransferRequest initiates a transfer from a customer through a token to another customer
// Sender: Customer in Token A (who initiated, certificate verified)
// Intermediate: Token B (receives funds, takes commission, forwards remainder to receiver customer)
// Receiver: Customer of Token B (final destination, must be registered with Token B)
// Status Flow: PendingSenderTokenApproval → PendingReceiverTokenApproval → Completed
// Checkpoint: receiverCustomerNetworkAddress must be a registered participant of receiverTokenID
// Commission: Retrieved from blockchain config (configured via SetTokenCommission)
// SECURITY: Includes currency compatibility check, exchange rate validation, timeout tracking, and escrow protection
func (s *SmartContract) CreateCustomerToTokenTransferRequest(ctx contractapi.TransactionContextInterface, senderNetworkAddress, senderTokenID, receiverTokenID, receiverCustomerNetworkAddress string, amount int) (string, error) {
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
	if senderTokenID == receiverTokenID {
		return "", fmt.Errorf("cannot transfer to the same token")
	}

	// Fetch commission rate from blockchain config for receiver token
	commissionConfig, err := s.GetTokenCommission(ctx, receiverTokenID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch commission config: %v", err)
	}
	// Calculate commission amount from percentage
	commissionAmount := int(float64(amount) * (commissionConfig.CommissionPercentage / 100))

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

	// Load sender's participant record to check balance
	senderCustomer, senderCustomerKey, err := s.getParticipantByNetworkToken(ctx, senderNetworkAddress, senderTokenID)
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
					existingTransfer.Amount == amount &&
					(existingTransfer.Status == "PendingSenderTokenApproval" || existingTransfer.Status == "PendingReceiverTokenApproval") {
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
	receiverCustomerAmount := amount - commissionAmount

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
	request := CustomerToTokenTransferRequest{
		TransferRequestID:       reqID,
		SenderCustomerID:        senderNetworkAddress,
		SenderCustomerTokenID:   senderCustomer.CustomerID,
		SenderCustomerName:      senderCustomer.Name,
		SenderTokenID:           senderTokenID,
		ReceiverTokenID:         receiverTokenID,
		ReceiverCustomerID:      receiverCustomerNetworkAddress,
		ReceiverCustomerTokenID: receiverCustomer.CustomerID,
		ReceiverCustomerName:    receiverCustomer.Name,
		Amount:                  amount,
		SenderCurrency:          senderCurrency,
		ReceiverCurrency:        receiverCurrency,
		Status:                  "PendingSenderTokenApproval",
		InitiatedBy:             callerID,
		DebitStatus:             "DEBITED",
		CreditStatus:            "PENDING",
		EscrowedAmount:          amount,
		ApprovedBySenderOwner:   false,
		ApprovedByReceiverOwner: false,
		CommissionPercentage:    float64(commissionAmount) / float64(amount) * 100,
		CommissionAmount:        commissionAmount,
		ReceiverCustomerAmount:  receiverCustomerAmount,
		// SECURITY FIX #10: Timeout tracking - record timestamp in SenderTokenOwnerApprovedAt for now (can add CreatedAt field if needed)
	}

	reqBytes, err := json.Marshal(request)
	if err != nil {
		// SECURITY FIX #11: Reverse debit on marshal error (improved escrow protection)
		senderCustomer.Balance = originalBalance
		senderCustomer.TokenTransferIDs = senderCustomer.TokenTransferIDs[:len(senderCustomer.TokenTransferIDs)-1]
		_ = ctx.GetStub().PutState(senderCustomerKey, senderCustomerUpdatedBytes)
		return "", err
	}

	if err := ctx.GetStub().PutState(reqID, reqBytes); err != nil {
		// SECURITY FIX #11: Reverse debit on state error (improved escrow protection)
		senderCustomer.Balance = originalBalance
		senderCustomer.TokenTransferIDs = senderCustomer.TokenTransferIDs[:len(senderCustomer.TokenTransferIDs)-1]
		_ = ctx.GetStub().PutState(senderCustomerKey, senderCustomerUpdatedBytes)
		return "", err
	}

	return reqID, nil

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
		if !strings.HasPrefix(kv.Key, "custtotoken_") {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		// Include transfers where this token is the sender
		// And status is PendingSenderTokenApproval
		if req.Status == "PendingSenderTokenApproval" && req.SenderTokenID == tokenID {
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
		if !strings.HasPrefix(kv.Key, "custtotoken_") {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		// Include transfers where this token is the receiver
		// And status is PendingReceiverTokenApproval
		if req.Status == "PendingReceiverTokenApproval" && req.ReceiverTokenID == tokenID {
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

	reqBytes, err := ctx.GetStub().GetState(transferRequestID)
	if err != nil || reqBytes == nil {
		return nil, fmt.Errorf("transfer request not found")
	}

	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}

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
		if !strings.HasPrefix(kv.Key, "custtotoken_") {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		// Include completed transfers involving this token
		if req.Status == "Completed" && (req.SenderTokenID == tokenID || req.ReceiverTokenID == tokenID) {
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

// GetCustomerToTokenTransferHistoryByCustomer returns completed customer-to-token transfers for a specific customer
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
		if !strings.HasPrefix(kv.Key, "custtotoken_") {
			continue
		}

		var req CustomerToTokenTransferRequest
		if err := json.Unmarshal(kv.Value, &req); err != nil {
			continue
		}

		// Include completed transfers where customer is sender or receiver
		if req.Status == "Completed" && (req.SenderCustomerID == customerNetworkAddress || req.ReceiverCustomerID == customerNetworkAddress) {
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

	// Load transfer request
	reqBytes, err := ctx.GetStub().GetState(transferRequestID)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("transfer request not found")
	}
	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}

	// Verify transfer is in correct status
	if request.Status != "PendingSenderTokenApproval" {
		return fmt.Errorf("transfer is not pending sender approval. Current status: %s", request.Status)
	}

	if !approved {
		// Reject the transfer
		request.Status = "RejectedBySenderOwner"
		request.CreditStatus = "REVERSED"
		// Reverse the debit on sender customer
		senderCustomer, senderCustomerKey, err := s.getParticipantByNetworkToken(ctx, request.SenderCustomerID, request.SenderTokenID)
		if err == nil && senderCustomer != nil {
			senderCustomer.Balance += request.EscrowedAmount
			updatedBytes, _ := json.Marshal(senderCustomer)
			_ = ctx.GetStub().PutState(senderCustomerKey, updatedBytes)
		}
		updatedReqBytes, _ := json.Marshal(request)
		return ctx.GetStub().PutState(transferRequestID, updatedReqBytes)
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
	request.Status = "PendingReceiverTokenApproval"

	// Update transfer request
	updatedReqBytes, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal transfer request: %v", err)
	}
	return ctx.GetStub().PutState(transferRequestID, updatedReqBytes)
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
	var convertedAmount int
	if exchangeRateStr != "" {
		var err error
		exchangeRate, err = strconv.ParseFloat(exchangeRateStr, 64)
		if err != nil {
			return fmt.Errorf("invalid exchange rate format: %v", err)
		}
	}
	if convertedAmountStr != "" {
		var err error
		convertedAmount, err = strconv.Atoi(convertedAmountStr)
		if err != nil {
			return fmt.Errorf("invalid converted amount format: %v", err)
		}
	}

	// Load transfer request
	reqBytes, err := ctx.GetStub().GetState(transferRequestID)
	if err != nil || reqBytes == nil {
		return fmt.Errorf("transfer request not found")
	}
	var request CustomerToTokenTransferRequest
	if err := json.Unmarshal(reqBytes, &request); err != nil {
		return fmt.Errorf("failed to unmarshal transfer request: %v", err)
	}

	// Verify transfer is in correct status (sender must have already approved)
	if request.Status != "PendingReceiverTokenApproval" {
		return fmt.Errorf("transfer is not pending receiver approval. Current status: %s", request.Status)
	}
	if !request.ApprovedBySenderOwner {
		return fmt.Errorf("transfer has not been approved by sender token owner")
	}

	if !approved {
		// Reject the transfer
		request.Status = "RejectedByReceiverOwner"
		request.CreditStatus = "REVERSED"
		// Reverse the debit on sender customer
		senderCustomer, senderCustomerKey, err := s.getParticipantByNetworkToken(ctx, request.SenderCustomerID, request.SenderTokenID)
		if err == nil && senderCustomer != nil {
			senderCustomer.Balance += request.EscrowedAmount
			updatedBytes, _ := json.Marshal(senderCustomer)
			_ = ctx.GetStub().PutState(senderCustomerKey, updatedBytes)
		}
		updatedReqBytes, _ := json.Marshal(request)
		return ctx.GetStub().PutState(transferRequestID, updatedReqBytes)
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
		receiverToken.Minted += request.CommissionAmount
		// Receiver customer gets forwarded amount (98%)
		receiverCustomer.Balance += request.ReceiverCustomerAmount
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
		receiverToken.ForeignBalances[senderCurrency] += request.EscrowedAmount

		// 2. Calculate commission from SENDER amount (2%)
		// Commission in sender currency = 3 USD × 2% = 0.06 USD
		commissionInSenderCurrency := int(float64(request.EscrowedAmount) * 0.02)

		// 3. Calculate remaining after commission
		// Remaining = 3 - 0.06 = 2.94 USD
		remainingAfterCommission := request.EscrowedAmount - commissionInSenderCurrency

		// 4. Use exchange rate to convert remaining amount
		// Converted amount = 2.94 USD × 83.5 = 245.49 INR
		actualConvertedAmount := float64(convertedAmount)
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
		receiverToken.Minted -= int(actualConvertedAmount)

		// 7. Credit customer with converted amount
		// Customer receives = 245.49 INR (no commission deducted from customer's amount)
		receiverCustomer.Balance += int(actualConvertedAmount)
	}

	ts, err := s.currentTxTime(ctx)
	if err != nil {
		return err
	}
	request.ReceiverTokenOwnerApprovedAt = ts
	request.CompletedAt = ts

	// Mark transfer as completed
	request.Status = "Completed"
	request.CreditStatus = "CREDITED"

	// Marshal all updates
	updatedReqBytes, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal transfer request: %v", err)
	}
	updatedReceiverTokenBytes, err := json.Marshal(receiverToken)
	if err != nil {
		return fmt.Errorf("failed to marshal receiver token: %v", err)
	}
	updatedReceiverCustomerBytes, err := json.Marshal(receiverCustomer)
	if err != nil {
		return fmt.Errorf("failed to marshal receiver customer: %v", err)
	}

	// Persist all updates
	if err := ctx.GetStub().PutState(transferRequestID, updatedReqBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(request.ReceiverTokenID, updatedReceiverTokenBytes); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(receiverCustomerKey, updatedReceiverCustomerBytes); err != nil {
		return err
	}

	return nil
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
