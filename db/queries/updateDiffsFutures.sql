WITH unique_pairs AS (
    SELECT DISTINCT symbol
    FROM pairsfutures
    WHERE markPrice <> 0 
      AND indexPrice <> 0
),
market_combinations AS (
    SELECT 
        a.symbol AS firstPairSymbol, 
        a.exchange AS firstPairExchange,
        a.market AS firstPairMarket,

        a.markPrice AS firstPairMarkPrice,
        a.indexPrice AS firstPairIndexPrice,

        a.baseVolume24h AS firstPairVolume,
        a.fundingRatePercent AS firstPairFundingRatePercent,
        a.baseAsset AS baseAsset,
        a.quoteAsset AS quoteAsset,

        b.symbol AS secondPairSymbol,
        b.exchange AS secondPairExchange,
        b.market AS secondPairMarket,

        b.markPrice AS secondPairMarkPrice,
        b.indexPrice AS secondPairIndexPrice,

        b.baseVolume24h AS secondPairVolume,
        b.fundingRatePercent AS secondPairFundingRatePercent

    FROM pairsfutures a
    JOIN pairsfutures b 
        -- ON a.symbol = b.symbol 
        -- AND a.exchange <> b.exchange

        -- ON a.symbol = b.symbol 
        ON a.exchange <> b.exchange
        AND a.baseAsset = b.baseAsset
        AND (
            a.quoteAsset = b.quoteAsset 
            OR (a.quoteAsset IN ('USDT', 'USDC') AND b.quoteAsset IN ('USDT', 'USDC'))
        )
        
    WHERE a.markPrice <> 0 
        AND a.indexPrice <> 0 
        AND b.markPrice <> 0 
        AND b.indexPrice <> 0
),
calculated_diffs AS (
    SELECT 
        CONCAT(firstPairSymbol, '_', secondPairSymbol) as symbol,
        CONCAT(firstPairSymbol, '_', secondPairSymbol, '_', firstPairExchange, '-', secondPairExchange) AS pairKey,
        firstPairExchange,
        firstPairMarket,
        ROUND(firstPairMarkPrice, 8) AS firstPairMarkPrice,
        ROUND(firstPairIndexPrice, 8) AS firstPairIndexPrice,
        ROUND(firstPairVolume, 2) AS firstPairVolume,
        secondPairExchange,
        secondPairMarket,
        baseAsset,
        quoteAsset,
        ROUND(secondPairMarkPrice, 8) AS secondPairMarkPrice,
        ROUND(secondPairIndexPrice, 8) AS secondPairIndexPrice,
        ROUND(secondPairVolume, 2) AS secondPairVolume,
        ROUND(secondPairMarkPrice - firstPairMarkPrice, 8) AS differenceMark,
        ROUND(secondPairIndexPrice - firstPairIndexPrice, 8) AS differenceIndex,
        CASE 
            WHEN TRUNC(((secondPairMarkPrice - firstPairMarkPrice) / firstPairMarkPrice) * 100, 2) > 1000000000 THEN 1000000000
            WHEN TRUNC(((secondPairMarkPrice - firstPairMarkPrice) / firstPairMarkPrice) * 100, 2) < -1000000000 THEN -1000000000
            ELSE TRUNC(((secondPairMarkPrice - firstPairMarkPrice) / firstPairMarkPrice) * 100, 2)
        END AS differenceMarkPercentage,
        CASE 
            WHEN TRUNC(((secondPairIndexPrice - firstPairIndexPrice) / firstPairIndexPrice) * 100, 2) > 1000000000 THEN 1000000000
            WHEN TRUNC(((secondPairIndexPrice - firstPairIndexPrice) / firstPairIndexPrice) * 100, 2) < -1000000000 THEN -1000000000
            ELSE TRUNC(((secondPairIndexPrice - firstPairIndexPrice) / firstPairIndexPrice) * 100, 2)
        END AS differenceIndexPercentage,
        firstPairFundingRatePercent,
        secondPairFundingRatePercent,
        ROUND((secondPairFundingRatePercent - firstPairFundingRatePercent), 6) AS differenceFundingRatePercent,
        (CASE 
            WHEN (firstPairFundingRatePercent > 0 AND secondPairFundingRatePercent < 0) 
              OR (firstPairFundingRatePercent < 0 AND secondPairFundingRatePercent > 0) 
            THEN TRUE 
            ELSE FALSE 
        END) AS isFundingRateOpposite,
        NOW() AT TIME ZONE 'UTC' AS updatedAt
    FROM market_combinations
),
updating_data AS (
    SELECT 
        d.pairKey,
        d.symbol,
        d.baseAsset,
        d.quoteAsset,
        d.firstPairExchange,
        d.firstPairMarket,
        d.firstPairMarkPrice,
        d.firstPairIndexPrice,
        d.firstPairVolume,
        d.secondPairExchange,
        d.secondPairMarket,
        d.secondPairMarkPrice,
        d.secondPairIndexPrice,
        d.secondPairVolume,
        d.differenceMark,
        d.differenceIndex,
        d.differenceMarkPercentage,
        d.differenceIndexPercentage,
        d.firstPairFundingRatePercent,
        d.secondPairFundingRatePercent,
        d.differenceFundingRatePercent,
        d.isFundingRateOpposite,
        '0 seconds' AS timeElapsed,
        '{}'::JSONB AS firstExchangeNetworks,
        '{}'::JSONB AS secondExchangeNetworks,
        NOW() AT TIME ZONE 'UTC' AS updatedAt
    FROM calculated_diffs d
    LEFT JOIN diffsfutures prev ON d.pairKey = prev.pairKey
)
INSERT INTO diffsfutures (
    pairKey, 
    symbol, 
    baseAsset, 
    quoteAsset, 
    firstPairExchange, 
    firstPairMarket, 
    firstPairMarkPrice, 
    firstPairIndexPrice, 
    firstPairVolume, 
    secondPairExchange, 
    secondPairMarket, 
    secondPairMarkPrice, 
    secondPairIndexPrice, 
    secondPairVolume, 
    differenceMark, 
    differenceIndex, 
    differenceMarkPercentage, 
    differenceIndexPercentage, 
    differenceFundingRatePercent, 
    firstPairFundingRate, 
    secondPairFundingRate,
    isFundingRateOpposite,
    firstExchangeNetworks, 
    secondExchangeNetworks, 
    timeElapsed, 
    updatedAt
)
SELECT 
    pairKey, 
    symbol, 
    baseAsset, 
    quoteAsset, 
    firstPairExchange, 
    firstPairMarket, 
    firstPairMarkPrice, 
    firstPairIndexPrice, 
    firstPairVolume, 
    secondPairExchange, 
    secondPairMarket, 
    secondPairMarkPrice, 
    secondPairIndexPrice, 
    secondPairVolume, 
    differenceMark, 
    differenceIndex, 
    differenceMarkPercentage, 
    differenceIndexPercentage, 
    differenceFundingRatePercent, 
    firstPairFundingRatePercent AS firstPairFundingRate, 
    secondPairFundingRatePercent AS secondPairFundingRate, 
    isFundingRateOpposite,
    '{}'::JSONB AS firstExchangeNetworks, 
    '{}'::JSONB AS secondExchangeNetworks, 
    '0 seconds'::INTERVAL AS timeElapsed, 
    updatedAt
FROM updating_data
ON CONFLICT (pairKey) DO UPDATE 
SET 
    firstPairMarkPrice = EXCLUDED.firstPairMarkPrice,
    firstPairIndexPrice = EXCLUDED.firstPairIndexPrice,
    firstPairVolume = EXCLUDED.firstPairVolume,
    secondPairMarkPrice = EXCLUDED.secondPairMarkPrice,
    secondPairIndexPrice = EXCLUDED.secondPairIndexPrice,
    secondPairVolume = EXCLUDED.secondPairVolume,
    differenceMark = EXCLUDED.differenceMark,
    differenceIndex = EXCLUDED.differenceIndex,
    differenceMarkPercentage = EXCLUDED.differenceMarkPercentage,
    differenceIndexPercentage = EXCLUDED.differenceIndexPercentage,
    differenceFundingRatePercent = EXCLUDED.differenceFundingRatePercent,
    firstPairFundingRate = EXCLUDED.firstPairFundingRate,
    secondPairFundingRate = EXCLUDED.secondPairFundingRate,
    isFundingRateOpposite = EXCLUDED.isFundingRateOpposite,
    firstExchangeNetworks = EXCLUDED.firstExchangeNetworks,
    secondExchangeNetworks = EXCLUDED.secondExchangeNetworks,
    timeElapsed = EXCLUDED.timeElapsed,
    updatedAt = NOW() AT TIME ZONE 'UTC';
