DROP TABLE IF EXISTS pairs CASCADE;
DROP TABLE IF EXISTS diffs CASCADE;
DROP TABLE IF EXISTS nets CASCADE;
DROP TABLE IF EXISTS pairsfutures CASCADE;
DROP TABLE IF EXISTS diffsfutures CASCADE;

CREATE TABLE pairs (
    id SERIAL PRIMARY KEY,
    pairKey VARCHAR(50) UNIQUE NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    exchange VARCHAR(20) NOT NULL,
    market VARCHAR(20) NOT NULL,
    price DECIMAL(18,8) NOT NULL,
    baseAsset VARCHAR(20) NOT NULL,
    quoteAsset VARCHAR(20) NOT NULL,
    displayName VARCHAR(20) NOT NULL,
    priceChangePercent24h DECIMAL(10,2) NOT NULL,
    baseVolume24h DECIMAL(20,2) NOT NULL,
    quoteVolume24h DECIMAL(20,2) NOT NULL,
    updatedAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    createdAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX symbol_idx ON pairs (symbol);
CREATE INDEX pairKey_idx ON pairs (pairKey);
CREATE INDEX exchange_market_idx ON pairs (exchange, market);

CREATE TABLE diffs (
    id SERIAL PRIMARY KEY,
    pairKey VARCHAR(60) UNIQUE NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    baseAsset VARCHAR(20) NOT NULL,
    quoteAsset VARCHAR(20) NOT NULL,
    firstPairExchange VARCHAR(20) NOT NULL,
    firstPairMarket VARCHAR(20) NOT NULL,
    firstPairPrice DECIMAL(20,8) NOT NULL,
    firstPairVolume DECIMAL(30,2) NOT NULL,
    secondPairExchange VARCHAR(20) NOT NULL,
    secondPairMarket VARCHAR(20) NOT NULL,
    secondPairPrice DECIMAL(20,8) NOT NULL,
    secondPairVolume DECIMAL(30,2) NOT NULL,
    difference DECIMAL(20,8) NOT NULL,
    differencePercentage DECIMAL(12,2) NOT NULL,
    firstExchangeNetworks JSONB DEFAULT '{}'::JSONB,
    secondExchangeNetworks JSONB DEFAULT '{}'::JSONB,
    timeOfLife TIMESTAMP NULL,
    timeElapsed INTERVAL NOT NULL DEFAULT '0 seconds',
    updatedAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    createdAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX diffs_symbol_idx ON diffs (symbol);
CREATE INDEX diffs_pairKey_idx ON diffs (pairKey);
CREATE INDEX diffs_differencePercentage_idx ON diffs (differencePercentage);

CREATE TABLE nets (
    id SERIAL PRIMARY KEY,
    coinKey VARCHAR(100) UNIQUE NOT NULL,
    coin VARCHAR(50) NOT NULL,
    exchange VARCHAR(20) NOT NULL,
    network VARCHAR(50) NOT NULL,
    networkName VARCHAR(50) NOT NULL,
    depositEnable BOOLEAN NOT NULL,
    withdrawEnable BOOLEAN NOT NULL,
    updatedAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX nets_coin_idx ON nets (coin);
CREATE INDEX nets_exchange_idx ON nets (exchange);
CREATE INDEX nets_network_idx ON nets (network);

CREATE TABLE pairsfutures (
    id SERIAL PRIMARY KEY,
    pairKey VARCHAR(50) UNIQUE NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    exchange VARCHAR(20) NOT NULL,
    market VARCHAR(20) NOT NULL,
    markPrice DECIMAL(18,8) NOT NULL,
    indexPrice DECIMAL(18,8) NOT NULL,
    baseAsset VARCHAR(20) NOT NULL,
    quoteAsset VARCHAR(20) NOT NULL,
    displayName VARCHAR(20) NOT NULL,
    fundingRatePercent DECIMAL(14,10) NOT NULL,
    nextFundingTimestamp BIGINT NOT NULL,
    priceChangePercent24h DECIMAL(10,2) NOT NULL,
    baseVolume24h DECIMAL(20,2) NOT NULL,
    quoteVolume24h DECIMAL(20,2) NOT NULL,
    updatedAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    createdAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX symbolFutures_idx ON pairsfutures (symbol);
CREATE INDEX pairKeyFutures_idx ON pairsfutures (pairKey);
CREATE INDEX exchange_market_futures_idx ON pairsfutures (exchange, market);

CREATE TABLE diffsfutures (
    id SERIAL PRIMARY KEY,
    pairKey VARCHAR(60) UNIQUE NOT NULL,
    symbol VARCHAR(40) NOT NULL,
    baseAsset VARCHAR(20) NOT NULL,
    quoteAsset VARCHAR(20) NOT NULL,
    firstPairExchange VARCHAR(20) NOT NULL,
    firstPairMarket VARCHAR(20) NOT NULL,
    firstPairMarkPrice DECIMAL(20,8) NOT NULL,
    firstPairIndexPrice DECIMAL(20,8) NOT NULL,
    firstPairVolume DECIMAL(30,2) NOT NULL,
    firstPairFundingRate DECIMAL(10,6) NOT NULL,
    secondPairExchange VARCHAR(20) NOT NULL,
    secondPairMarket VARCHAR(20) NOT NULL,
    secondPairMarkPrice DECIMAL(20,8) NOT NULL,
    secondPairIndexPrice DECIMAL(20,8) NOT NULL,
    secondPairVolume DECIMAL(30,2) NOT NULL,
    secondPairFundingRate DECIMAL(10,6) NOT NULL,
    differenceMark DECIMAL(20,8) NOT NULL,
    differenceIndex DECIMAL(20,8) NOT NULL,
    differenceMarkPercentage DECIMAL(12,2) NOT NULL,
    differenceIndexPercentage DECIMAL(12,2) NOT NULL,
    differenceFundingRatePercent DECIMAL(10,6) NOT NULL,
    isFundingRateOpposite BOOLEAN NOT NULL DEFAULT FALSE,
    firstExchangeNetworks JSONB DEFAULT '{}'::JSONB,
    secondExchangeNetworks JSONB DEFAULT '{}'::JSONB,
    timeOfLife TIMESTAMP NULL,
    timeElapsed INTERVAL NOT NULL DEFAULT '0 seconds',
    updatedAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    createdAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX diffs_symbol_futures_idx ON diffsfutures (symbol);
CREATE INDEX diffs_pairKey_futures_idx ON diffsfutures (pairKey);
