Tables:
    1. Get prices and volumes for every pair from different CEX every 10 sec from api requests and write to postgreSQL table:
        pairsList: {
            key: symbol_exchange_market, 
            symbol: 'BTCUSDT',
            exchange: 'Binance',
            market: 'spot',
            price: 912345.67,
            baseAsset: 'BTC,
            quoteAsset: 'USDT',
            displayName: 'BTC/USDT',
            priceChangePercent24h: -1.23,
            baseVolume24h: 123,
            quoteVolume24h: 12345,
        }

    2. List of every unique pair and add markets that they work with as an array run as job every hour:
        listOfUniquePairs: {
            coin: 'BTCUSDT',
            market: ['Binance', 'Bybit', 'Huobi', ...]
        }

    3. Coin withdrawal and deposit network accessibility get from API and write to postgreSQL table, run as a job every hour:  
        coinNetworks: {
            "key": ?,
            "coin": "ETH",
            "market": "Binance",
            "depositAllEnable": true,
            "withdrawAllEnable": true,
            "networkList": [
                {
                    "network": "ETH",
                    "coin": "ETH",
                    "withdrawEnable": true,
                    "depositEnable": true,
                    "withdrawFee": "0.002",
                    "withdrawMin": "0.01",
                    "withdrawMax": "100",
                    "minConfirm": 12,
                    "unLockConfirm": 24,
                    "name": "Ethereum",
                    "resetAddressStatus": false,
                    "addressRegex": "^(0x)[0-9A-Fa-f]{40}$",
                    "memoRegex": "",
                    "specialTips": "",
                    "contractAddress": "",
                    "sameAddress": false
                }
            ]
        }
    4. Market inforamtion (simple table, no API and jobs)
        maarketsData: {
            name: 'Binance',
        }

    5. Table to compare prices run every 10 seconds
        1. Take all unique pairs like BTCUSDT from listOfUniquePairs

        2. Find all possible market combinations for every unique pair: 
            BTCUSDT (Binance) - BTCUSDT (Bybit)
            BTCUSDT (Binance) - BTCUSDT (Huobi)
            BTCUSDT (Bybit) - BTCUSDT (Huobi)
            BTCUSDT (Bybit) - BTCUSDT (Binance)
            BTCUSDT (Huobi) - BTCUSDT (Binance)
            BTCUSDT (Huobi) - BTCUSDT (Bybit)

        3. Calculate price difference using price data from pairsList like: 
            [
                {
                    key: BTCUSDT_Binance_spot-BTCUSDT_Bybit_spot, 
                    pairs: BTCUSDT_Binance_spot, BTCUSDT_Bybit_spot, 
                    firstPairPrice: 91234.56,
                    firstPairMarket: 'Binance',
                    firstPairVolume: 3456789,
                    secondPairPrice: 92345.67,
                    secondPairMarket: 'Bybit',
                    secondPairVolume: 2345678,
                    type: 'spot,
                    baseAsset: 'BTC,
                    quoteAsset: 'USDT',
                    displayName: 'BTC/USDT',
                    difference: 1,111.11,
                    differencePercentage: 1.21,
                },
                {
                    key: BTCUSDT_Bybit_spot-BTCUSDT_Binance_spot, 
                    pairs: BTCUSDT_Bybit_spot, BTCUSDT_Binance_spot, 
                    firstPairPrice: 92345.67,
                    firstPairMarket: 'Bybit',
                    firstPairVolume: 2345678,
                    secondPairPrice: 91234.56,
                    secondPairMarket: 'Binance',
                    secondPairVolume: 3456789,
                    type: 'spot,
                    baseAsset: 'BTC,
                    quoteAsset: 'USDT',
                    displayName: 'BTC/USDT',
                    difference: -1,111.11,
                    differencePercentage: -1.21,
                },
            ]
        
        4. Add allowed network field as array of objects for every calculated pair in the list
            (
                check for 
                key: BTCUSDT_Binance_spot-BTCUSDT_Bybit_spot, 
                what networks uses BTC coin on Binance and BTC coin on Bybit
                and write to the list and check is both are available to use 
                check in coinNetworks table in networkList array
                i need to be able to understand if i can withdraw coin from first exchange 
                and send it to the second exchange if in both exchanges the same network is available
            )





Huobi - Skipping pairs