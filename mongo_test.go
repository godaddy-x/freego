package main

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/goquery"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/utils/gauth"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"net/url"
	"testing"
	"time"
)

func init() {
	initMongoDB()
}

// 智能合约
type OwContract struct {
	Id         int64  `json:"id" bson:"_id"`
	MainSymbol string `json:"mainSymbol" bson:"mainSymbol"` // 基础分类-> mainSymbol
	Symbol     string `json:"symbol" bson:"symbol"`         //主链标记
	Name       string `json:"name" bson:"name"`
	Decimals   int64  `json:"decimals" bson:"decimals"`
	Address    string `json:"address" bson:"address"`
	ContractID string `json:"contractID" bson:"contractID"`
	Token      string `json:"token" bson:"token"` //token标记
	Protocol   string `json:"protocol" bson:"protocol"`
	ABI        string `json:"abi" bson:"abi"`
	Ctime      int64  `json:"ctime" bson:"ctime"`
	State      int64  `json:"state" bson:"state"`
}

func (o *OwContract) GetTable() string {
	return "ow_contract"
}

func (o *OwContract) NewObject() sqlc.Object {
	return &OwContract{}
}

func (o *OwContract) NewIndex() []sqlc.Index {
	symbol := sqlc.Index{Name: "symbol", Key: []string{"symbol"}}
	address := sqlc.Index{Name: "address", Key: []string{"address"}}
	contractID := sqlc.Index{Name: "contractID", Key: []string{"contractID"}, Unique: true}
	many := sqlc.Index{Name: "many", Key: []string{"symbol", "address", "contractID"}, Unique: true}
	return []sqlc.Index{symbol, address, contractID, many}
}

func TestMongoIndex(t *testing.T) {
	//db, err := sqld.NewMongo()
	//if err != nil {
	//	panic(err)
	//}
	//defer db.Close()
	//coll, err := db.GetDatabase(new(OwContract).GetTable())
	//if err != nil {
	//	panic(err)
	//}
	//cur, err := coll.Indexes().List(context.Background())
	//if err != nil {
	//	panic(err)
	//}
	//var index []sqlc.Index
	//if err := cur.All(context.Background(), &index); err != nil {
	//	panic(err)
	//}
	//a, _ := utils.JsonMarshal(index)
	//fmt.Println(string(a))

	sqld.RebuildMongoDBIndex()
}

func TestMongoSave(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	//l := utils.UnixMilli()
	o := OwContract{
		MainSymbol: "ETH",
		Symbol:     "MATIC",
		Address:    "0x9866c45224667061f8c9e66db38d9316a8d68951",
		Token:      "NFTSWAP",
		Name:       "NFTSWAP",
		Ctime:      utils.UnixMilli(),
		ABI:        `[{"inputs":[{"internalType":"uint256","name":"chainId","type":"uint256"}],"payable":false,"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"bytes4","name":"id","type":"bytes4"},{"indexed":false,"internalType":"address","name":"assetProxy","type":"address"}],"name":"AssetProxyRegistered","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"makerAddress","type":"address"},{"indexed":true,"internalType":"address","name":"feeRecipientAddress","type":"address"},{"indexed":false,"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"indexed":false,"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"indexed":false,"internalType":"address","name":"senderAddress","type":"address"},{"indexed":true,"internalType":"bytes32","name":"orderHash","type":"bytes32"}],"name":"Cancel","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"makerAddress","type":"address"},{"indexed":true,"internalType":"address","name":"orderSenderAddress","type":"address"},{"indexed":false,"internalType":"uint256","name":"orderEpoch","type":"uint256"}],"name":"CancelUpTo","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"makerAddress","type":"address"},{"indexed":true,"internalType":"address","name":"feeRecipientAddress","type":"address"},{"indexed":false,"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"indexed":false,"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"indexed":false,"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"indexed":false,"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"},{"indexed":true,"internalType":"bytes32","name":"orderHash","type":"bytes32"},{"indexed":false,"internalType":"address","name":"takerAddress","type":"address"},{"indexed":false,"internalType":"address","name":"senderAddress","type":"address"},{"indexed":false,"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"indexed":false,"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"indexed":false,"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"indexed":false,"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"indexed":false,"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"name":"Fill","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"previousOwner","type":"address"},{"indexed":true,"internalType":"address","name":"newOwner","type":"address"}],"name":"OwnershipTransferred","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"address","name":"oldProtocolFeeCollector","type":"address"},{"indexed":false,"internalType":"address","name":"updatedProtocolFeeCollector","type":"address"}],"name":"ProtocolFeeCollectorAddress","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"uint256","name":"oldProtocolFeeMultiplier","type":"uint256"},{"indexed":false,"internalType":"uint256","name":"updatedProtocolFeeMultiplier","type":"uint256"}],"name":"ProtocolFeeMultiplier","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"signerAddress","type":"address"},{"indexed":true,"internalType":"address","name":"validatorAddress","type":"address"},{"indexed":false,"internalType":"bool","name":"isApproved","type":"bool"}],"name":"SignatureValidatorApproval","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"bytes32","name":"transactionHash","type":"bytes32"}],"name":"TransactionExecution","type":"event"},{"constant":true,"inputs":[],"name":"EIP1271_MAGIC_VALUE","outputs":[{"internalType":"bytes4","name":"","type":"bytes4"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[],"name":"EIP712_EXCHANGE_DOMAIN_HASH","outputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[{"internalType":"address","name":"","type":"address"},{"internalType":"address","name":"","type":"address"}],"name":"allowedValidators","outputs":[{"internalType":"bool","name":"","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"}],"name":"batchCancelOrders","outputs":[],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"gasPrice","type":"uint256"},{"internalType":"address","name":"signerAddress","type":"address"},{"internalType":"bytes","name":"data","type":"bytes"}],"internalType":"struct LibZeroExTransaction.ZeroExTransaction[]","name":"transactions","type":"tuple[]"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"batchExecuteTransactions","outputs":[{"internalType":"bytes[]","name":"returnData","type":"bytes[]"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"},{"internalType":"uint256[]","name":"takerAssetFillAmounts","type":"uint256[]"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"batchFillOrKillOrders","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults[]","name":"fillResults","type":"tuple[]"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"},{"internalType":"uint256[]","name":"takerAssetFillAmounts","type":"uint256[]"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"batchFillOrders","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults[]","name":"fillResults","type":"tuple[]"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"},{"internalType":"uint256[]","name":"takerAssetFillAmounts","type":"uint256[]"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"batchFillOrdersNoThrow","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults[]","name":"fillResults","type":"tuple[]"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"leftOrders","type":"tuple[]"},{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"rightOrders","type":"tuple[]"},{"internalType":"bytes[]","name":"leftSignatures","type":"bytes[]"},{"internalType":"bytes[]","name":"rightSignatures","type":"bytes[]"}],"name":"batchMatchOrders","outputs":[{"components":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults[]","name":"left","type":"tuple[]"},{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults[]","name":"right","type":"tuple[]"},{"internalType":"uint256","name":"profitInLeftMakerAsset","type":"uint256"},{"internalType":"uint256","name":"profitInRightMakerAsset","type":"uint256"}],"internalType":"struct LibFillResults.BatchMatchedFillResults","name":"batchMatchedFillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"leftOrders","type":"tuple[]"},{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"rightOrders","type":"tuple[]"},{"internalType":"bytes[]","name":"leftSignatures","type":"bytes[]"},{"internalType":"bytes[]","name":"rightSignatures","type":"bytes[]"}],"name":"batchMatchOrdersWithMaximalFill","outputs":[{"components":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults[]","name":"left","type":"tuple[]"},{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults[]","name":"right","type":"tuple[]"},{"internalType":"uint256","name":"profitInLeftMakerAsset","type":"uint256"},{"internalType":"uint256","name":"profitInRightMakerAsset","type":"uint256"}],"internalType":"struct LibFillResults.BatchMatchedFillResults","name":"batchMatchedFillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"order","type":"tuple"}],"name":"cancelOrder","outputs":[],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"internalType":"uint256","name":"targetOrderEpoch","type":"uint256"}],"name":"cancelOrdersUpTo","outputs":[],"payable":true,"stateMutability":"payable","type":"function"},{"constant":true,"inputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"name":"cancelled","outputs":[{"internalType":"bool","name":"","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[],"name":"currentContextAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[],"name":"detachProtocolFeeCollector","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"gasPrice","type":"uint256"},{"internalType":"address","name":"signerAddress","type":"address"},{"internalType":"bytes","name":"data","type":"bytes"}],"internalType":"struct LibZeroExTransaction.ZeroExTransaction","name":"transaction","type":"tuple"},{"internalType":"bytes","name":"signature","type":"bytes"}],"name":"executeTransaction","outputs":[{"internalType":"bytes","name":"","type":"bytes"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"order","type":"tuple"},{"internalType":"uint256","name":"takerAssetFillAmount","type":"uint256"},{"internalType":"bytes","name":"signature","type":"bytes"}],"name":"fillOrKillOrder","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"fillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"order","type":"tuple"},{"internalType":"uint256","name":"takerAssetFillAmount","type":"uint256"},{"internalType":"bytes","name":"signature","type":"bytes"}],"name":"fillOrder","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"fillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":true,"inputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"name":"filled","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[{"internalType":"bytes4","name":"assetProxyId","type":"bytes4"}],"name":"getAssetProxy","outputs":[{"internalType":"address","name":"assetProxy","type":"address"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"order","type":"tuple"}],"name":"getOrderInfo","outputs":[{"components":[{"internalType":"enum LibOrder.OrderStatus","name":"orderStatus","type":"uint8"},{"internalType":"bytes32","name":"orderHash","type":"bytes32"},{"internalType":"uint256","name":"orderTakerAssetFilledAmount","type":"uint256"}],"internalType":"struct LibOrder.OrderInfo","name":"orderInfo","type":"tuple"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[{"internalType":"bytes32","name":"hash","type":"bytes32"},{"internalType":"address","name":"signerAddress","type":"address"},{"internalType":"bytes","name":"signature","type":"bytes"}],"name":"isValidHashSignature","outputs":[{"internalType":"bool","name":"isValid","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"order","type":"tuple"},{"internalType":"bytes","name":"signature","type":"bytes"}],"name":"isValidOrderSignature","outputs":[{"internalType":"bool","name":"isValid","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[{"components":[{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"gasPrice","type":"uint256"},{"internalType":"address","name":"signerAddress","type":"address"},{"internalType":"bytes","name":"data","type":"bytes"}],"internalType":"struct LibZeroExTransaction.ZeroExTransaction","name":"transaction","type":"tuple"},{"internalType":"bytes","name":"signature","type":"bytes"}],"name":"isValidTransactionSignature","outputs":[{"internalType":"bool","name":"isValid","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"},{"internalType":"uint256","name":"makerAssetFillAmount","type":"uint256"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"marketBuyOrdersFillOrKill","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"fillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"},{"internalType":"uint256","name":"makerAssetFillAmount","type":"uint256"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"marketBuyOrdersNoThrow","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"fillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"},{"internalType":"uint256","name":"takerAssetFillAmount","type":"uint256"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"marketSellOrdersFillOrKill","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"fillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order[]","name":"orders","type":"tuple[]"},{"internalType":"uint256","name":"takerAssetFillAmount","type":"uint256"},{"internalType":"bytes[]","name":"signatures","type":"bytes[]"}],"name":"marketSellOrdersNoThrow","outputs":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"fillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"leftOrder","type":"tuple"},{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"rightOrder","type":"tuple"},{"internalType":"bytes","name":"leftSignature","type":"bytes"},{"internalType":"bytes","name":"rightSignature","type":"bytes"}],"name":"matchOrders","outputs":[{"components":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"left","type":"tuple"},{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"right","type":"tuple"},{"internalType":"uint256","name":"profitInLeftMakerAsset","type":"uint256"},{"internalType":"uint256","name":"profitInRightMakerAsset","type":"uint256"}],"internalType":"struct LibFillResults.MatchedFillResults","name":"matchedFillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"leftOrder","type":"tuple"},{"components":[{"internalType":"address","name":"makerAddress","type":"address"},{"internalType":"address","name":"takerAddress","type":"address"},{"internalType":"address","name":"feeRecipientAddress","type":"address"},{"internalType":"address","name":"senderAddress","type":"address"},{"internalType":"uint256","name":"makerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetAmount","type":"uint256"},{"internalType":"uint256","name":"makerFee","type":"uint256"},{"internalType":"uint256","name":"takerFee","type":"uint256"},{"internalType":"uint256","name":"expirationTimeSeconds","type":"uint256"},{"internalType":"uint256","name":"salt","type":"uint256"},{"internalType":"bytes","name":"makerAssetData","type":"bytes"},{"internalType":"bytes","name":"takerAssetData","type":"bytes"},{"internalType":"bytes","name":"makerFeeAssetData","type":"bytes"},{"internalType":"bytes","name":"takerFeeAssetData","type":"bytes"}],"internalType":"struct LibOrder.Order","name":"rightOrder","type":"tuple"},{"internalType":"bytes","name":"leftSignature","type":"bytes"},{"internalType":"bytes","name":"rightSignature","type":"bytes"}],"name":"matchOrdersWithMaximalFill","outputs":[{"components":[{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"left","type":"tuple"},{"components":[{"internalType":"uint256","name":"makerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"takerAssetFilledAmount","type":"uint256"},{"internalType":"uint256","name":"makerFeePaid","type":"uint256"},{"internalType":"uint256","name":"takerFeePaid","type":"uint256"},{"internalType":"uint256","name":"protocolFeePaid","type":"uint256"}],"internalType":"struct LibFillResults.FillResults","name":"right","type":"tuple"},{"internalType":"uint256","name":"profitInLeftMakerAsset","type":"uint256"},{"internalType":"uint256","name":"profitInRightMakerAsset","type":"uint256"}],"internalType":"struct LibFillResults.MatchedFillResults","name":"matchedFillResults","type":"tuple"}],"payable":true,"stateMutability":"payable","type":"function"},{"constant":true,"inputs":[{"internalType":"address","name":"","type":"address"},{"internalType":"address","name":"","type":"address"}],"name":"orderEpoch","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[],"name":"owner","outputs":[{"internalType":"address","name":"","type":"address"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"internalType":"bytes32","name":"hash","type":"bytes32"}],"name":"preSign","outputs":[],"payable":true,"stateMutability":"payable","type":"function"},{"constant":true,"inputs":[{"internalType":"bytes32","name":"","type":"bytes32"},{"internalType":"address","name":"","type":"address"}],"name":"preSigned","outputs":[{"internalType":"bool","name":"","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[],"name":"protocolFeeCollector","outputs":[{"internalType":"address","name":"","type":"address"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[],"name":"protocolFeeMultiplier","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"internalType":"address","name":"assetProxy","type":"address"}],"name":"registerAssetProxy","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"internalType":"address","name":"updatedProtocolFeeCollector","type":"address"}],"name":"setProtocolFeeCollectorAddress","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"internalType":"uint256","name":"updatedProtocolFeeMultiplier","type":"uint256"}],"name":"setProtocolFeeMultiplier","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"internalType":"address","name":"validatorAddress","type":"address"},{"internalType":"bool","name":"approval","type":"bool"}],"name":"setSignatureValidatorApproval","outputs":[],"payable":true,"stateMutability":"payable","type":"function"},{"constant":false,"inputs":[{"internalType":"bytes[]","name":"assetData","type":"bytes[]"},{"internalType":"address[]","name":"fromAddresses","type":"address[]"},{"internalType":"address[]","name":"toAddresses","type":"address[]"},{"internalType":"uint256[]","name":"amounts","type":"uint256[]"}],"name":"simulateDispatchTransferFromCalls","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":true,"inputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"name":"transactionsExecuted","outputs":[{"internalType":"bool","name":"","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"internalType":"address","name":"newOwner","type":"address"}],"name":"transferOwnership","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"}]`,
		State:      1,
	}
	if err := db.Save(&o); err != nil {
		fmt.Println(err)
	}
}

func TestMongoUpdate(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	objectID, err := primitive.ObjectIDFromHex("63462950d9180b6c9c0731f3")
	if err != nil {
		panic(err)
	}
	if err := db.Update(&OwWallet2{Id: objectID, WalletID: "aaaa"}); err != nil {
		panic(err)
	}

}

func TestMongoUpdateByCnd1(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Or(sqlc.M().In("id", 1577924742445268992), sqlc.M().Eq("id", 1577932141914750978)).Upset([]string{"appID", "ctime"}, "test1test1", 123)); err != nil {
		fmt.Println(err)
	}
	//fmt.Println(wallet.Id)
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMongoDelete(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.Delete(&OwWallet{Id: 1577899616857227264}); err != nil {
		fmt.Println(err)
	}
}

func TestMongoAgg(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	db.FindOne(sqlc.M().Agg(sqlc.SUM_, "paidPrice").Groupby("shopId", "userId").Asc("shopId").Limit(1, 1), &OwWallet{})
	//db.FindOne(sqlc.M().Groupby("appID"), &OwWallet{})
}

func TestMongoCount(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if c, err := db.Count(sqlc.M(&OwWallet{})); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
}

func TestMongoFindList(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	var o []*OwWallet
	sql := sqlc.M(&OwWallet{}).Collation(&sqlc.Collation{NumericOrdering: true}).Orderby("keyJson", sqlc.ASC_).Limit(0, 3)
	if err := db.FindList(sql, &o); err != nil {
		fmt.Println(err)
	}
	fmt.Println(sql.Pagination)
	for _, v := range o {
		fmt.Println("result: ", v.Id, v.Keystore)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMongoFindOne(t *testing.T) {
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	o := &OwWallet2{}
	if err := db.FindOne(sqlc.M().Fields("id", "appID").Orderby("id", sqlc.DESC_), o); err != nil {
		fmt.Println(err)
	}
	fmt.Println(o.Id.Hex())
}

func TestMongoUpdateByCnd(t *testing.T) {
	l := utils.UnixMilli()
	db, err := sqld.NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	objectID, err := primitive.ObjectIDFromHex("63462950d9180b6c9c0731f3")
	if err != nil {
		panic(err)
	}
	if err := db.UpdateByCnd(sqlc.M(&OwWallet2{}).Eq("id", objectID).Upset([]string{"password", "rootPath"}, "123456test", "/test/123")); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMongoTx(t *testing.T) {
	err := sqld.UseTransaction(func(self *sqld.MGOManager) error {
		o := OwWallet{
			AppID:    utils.NextSID(),
			WalletID: utils.NextSID(),
		}
		if err := self.Save(&o); err != nil {
			return err
		}
		if err := self.Update(&o); err != nil { // 抛出异常,没有查询到数据
			return err
		}
		return utils.Error("test save error")
	})
	if err != nil {
		log.Fatal(err)
	}
}

func TestRedis(t *testing.T) {
	client, err := new(cache.RedisManager).Client()
	if err != nil {
		fmt.Println(err.Error())
	} else {
		client.Put("redislock:test", "1", 60)
		//if err := client.TryLock("test", func() error {
		//	return nil
		//}); err != nil {
		//	fmt.Println(err.Error())
		//}
		//client.Del("test")
		s := ""
		fmt.Println(client.Get("redislock:test", &s))
		fmt.Println(s)
		fmt.Println(client.Size("tx.block.coin.BTC"))
	}
}

func TestEX(t *testing.T) {
	x := utils.UnixMilli()
	fmt.Println(x)
	fmt.Println(utils.GetFmtDate(x))
}

func TestGA(t *testing.T) {
	// 生成种子
	seed := gauth.GenerateSeed()
	fmt.Println("种子: ", seed)
	// 通过种子生成密钥
	key, _ := gauth.GenerateSecretKey(seed)
	fmt.Println("密钥: ", key)
	// 通过密钥+时间生成验证码
	rs := gauth.GetNewCode(key, time.Now().Unix())
	fmt.Println("验证码: ", rs)
	fmt.Println("开始睡眠延迟中,请耐心等待...")
	time.Sleep(5 * time.Second)
	// 校验已有验证码
	fmt.Println("校验结果: ", gauth.ValidCode(key, rs))
}

func TestWebsocket_client_login(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":9090", Path: "/login2"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go goLogin(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

func TestWebsocket_client_logout(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":9090", Path: "/logout2"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go goLogout(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

func TestWebsocket_client_test(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":9090", Path: "/test2"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go goTest(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

var token = "eyJub2QiOjAsInR5cCI6IkpXVCIsImFsZyI6IlNIQTI1NiJ9.eyJzdWIiOiJ6aGFuZ3NhbiIsImRldiI6IiIsImF1ZCI6IjEyNy4wLjAuMSIsImlzcyI6IjQ1NiIsImlhdCI6MTU0ODc1MzQ2NjY0NCwiZXhwIjoxNTQ4NzU1MjY2NjQ0LCJyeHAiOjE1NDg3NTUyNjY2NDQsIm5iZiI6MTU0ODc1MzQ2NjY0NCwianRpIjoiNWVlOWMzZmM1ODUzNWRlOWMwMWIzZDIyOTAyNDExZTIxYWI0ODQ5NDRhMzAwYzkxOTg5NzI3Mjk1ZDYwNWI3NSIsImV4dCI6e319.c66ccbfb3eb7a95aaf2a981570990a645a77f7e89df8b46b0c744355f06bdb59"

func goTest(conn *websocket.Conn) {
	for {
		time.Sleep(time.Second * 1)
		data := map[string]interface{}{"token": token}
		send, _ := utils.JsonMarshal(&data)
		conn.WriteMessage(websocket.TextMessage, send)
	}
}

func goLogin(conn *websocket.Conn) {
	time.Sleep(time.Second * 1)
	data := map[string]interface{}{"user": "zhangsan"}
	send, _ := utils.JsonMarshal(&data)
	conn.WriteMessage(websocket.TextMessage, send)
}

func goLogout(conn *websocket.Conn) {
	time.Sleep(time.Second * 1)
	data := map[string]interface{}{"token": token}
	send, _ := utils.JsonMarshal(&data)
	conn.WriteMessage(websocket.TextMessage, send)
}

func TestWS(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":8080", Path: "/echo"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go timeWriter(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

func timeWriter(conn *websocket.Conn) {
	//for {
	time.Sleep(time.Second * 2)
	conn.WriteMessage(websocket.TextMessage, []byte(time.Now().Format("2006-01-02 15:04:05")))
	//}
}

func TestSorter(t *testing.T) {
	type Obj struct {
		Key string
		Val int64
	}
	d := []interface{}{
		&Obj{"e", 10},
		&Obj{"a", 2},
		&Obj{"d", 15},
		&Obj{"c", 8},
		&Obj{"f", 1},
		&Obj{"b", 12},
	}
	result := concurrent.NewSorter(d, func(o1, o2 interface{}) bool {
		a := o1.(*Obj)
		b := o2.(*Obj)
		return a.Val < b.Val // 判断值大小排序
	}).Sort()
	for _, v := range result {
		fmt.Println(v)
	}
}

func TestB(t *testing.T) {
	l := utils.UnixMilli()
	s := "mydata4vipday.720.datx"
	for i := 0; i < 1000000000; i++ {
		a := utils.Str2Bytes(s)
		_ = utils.Bytes2Str(a)
	}
	fmt.Println(utils.UnixMilli() - l)
}

func TestRGX1(t *testing.T) {
	//var aeskey = "x4kkptzFsUOVnuya"
	//fmt.Println("密钥: ", aeskey)
	//pass := "123456"
	//xpass := utils.AesEncrypt(pass, aeskey)
	//
	//fmt.Printf("加密后:%v\n", xpass)
	//
	//tpass := utils.AesDecrypt(xpass, string(aeskey))
	//fmt.Printf("解密后:%s\n", tpass)

	//s := "(?i)eval\\s*\\((.*?)\\)"
	//c := "<eval  ()"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "(?i)expression\\s*\\((.*?)\\)"
	//c := "<ExpressiOn  ()"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "(?i)scrip1t\\s*\\>(.*?)"
	//c := "<scRip1t  >"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "javascript:(.*?)|vbscript\\s*\\:(.*?)|view-source\\s*\\:(.*?)"
	//c := "vbscript:vbscript"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "\\{1}"
	//c := `\`
	//fmt.Println(utils.ValidPattern(c, s))
	fmt.Println(fmt.Sprintf("%x", `%`))
	fmt.Println(url.QueryEscape("%"))

}

type HtmlValidResult struct {
	NewData    string
	ContentLen int
}

var htmlstr = `
<section style="text-align: center; color: rgb(68, 198, 123); font-weight: 800; font-style: italic; text-decoration: line-through;">&nbsp;&nbsp;&lt;a&gt;斯蒂芬  撒旦法毒贩夫妇%3Csectioin&lt;/a&gt;%#\'":;.img src='test'/></section>
<h2 style="text-align: center; color: rgb(68, 198, 123); font-weight: 800; font-style: italic; text-decoration: line-through;">&nbsp;&nbsp;&lt;a&gt;斯蒂芬撒旦法毒贩夫妇%3Csectioin&lt;/a&gt;%#\'":;.img src='test'/></h2>
`

func TestHtml(t *testing.T) {
	//valid := goquery.ValidZxHtml(htmlstr)
	//fmt.Println(valid.ContentLen, valid.NewContent, valid.FailMsg)
	s := "//static.pgwjc.com/skin/images/1180654561279344640/1243153846100819968.jpg"
	fmt.Println(goquery.ValidImgURL(s, "//static.pgwjc.com/skin/images/"))
}

func TestRGX2(t *testing.T) {
	m := map[string]int{"test": 1}
	b, _ := utils.JsonMarshal(m)
	r := map[string]interface{}{}
	utils.JsonUnmarshal(b, &r)
	fmt.Println(len("1566722843972"))
	//x := "世界上最邪恶最专制的现代奴隶制国家--朝鲜"
	//key :=utils.Substr( utils.MD5("hgfedcba87654321"), 0, 16)
	//x1 := utils.AesEncrypt(x, key)
	//fmt.Println(x1)
	//x2 := utils.AesDecrypt(x1, key)
	//fmt.Print(string(x2))
	//start := utils.UnixMilli()
	//for i := 0; i < 20000; i++ {
	//	// MyClient.Connect(context.Background())
	//	c := MyClient.Database("openwallet").Collection("ow_wallet")
	//	pipeline := []map[string]interface{}{{"$match": map[string]interface{}{"_id": 8266}}}
	//	//pipeline := []map[string]interface{}{}
	//	batchSize := int32(5)
	//	cursor, err := c.Aggregate(context.Background(), pipeline, &options.AggregateOptions{BatchSize: &batchSize})
	//	if err != nil {
	//		fmt.Println(err)
	//		return
	//	}
	//	for cursor.Next(context.Background()) {
	//		//fmt.Println(cursor.Current.String())
	//		o := OwWallet{}
	//		cursor.Decode(&o)
	//		//fmt.Println(o)
	//	}
	//	// MyClient.Disconnect(context.Background())
	//}
	//fmt.Println("cost: ", utils.UnixMilli()-start)
}

//func BenchmarkLoopsParallel(b *testing.B) {
//	i := float64(1)
//	// b.SetParallelism(1000)
//	b.N = 50000
//	b.ReportAllocs()
//	b.RunParallel(func(pb *testing.PB) { //并发
//		for pb.Next() {
//			s := 145647.454564
//			s = s + i
//			utils.Shift(s, 10, true)
//		}
//	})
//}
