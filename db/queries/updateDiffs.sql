WITH unique_pairs AS (
    SELECT DISTINCT symbol
    FROM pairs
    WHERE price <> 0
),
market_combinations AS (
    SELECT 
        a.symbol,
        a.exchange AS firstPairExchange,
        a.market AS firstPairMarket,
        a.price AS firstPairPrice,
        a.baseVolume24h AS firstPairVolume,
        a.baseAsset AS baseAsset,
        a.quoteAsset AS quoteAsset,
        b.exchange AS secondPairExchange,
        b.market AS secondPairMarket,
        b.price AS secondPairPrice,
        b.baseVolume24h AS secondPairVolume
    FROM pairs a
    JOIN pairs b 
        ON a.symbol = b.symbol 
        AND a.exchange <> b.exchange
    WHERE a.price <> 0 AND b.price <> 0
),
calculated_diffs AS (
    SELECT 
        symbol,
        CONCAT(symbol, '_', firstPairExchange, '-', secondPairExchange) AS pairKey,
        firstPairExchange,
        firstPairMarket,
        ROUND(firstPairPrice, 8) AS firstPairPrice,
        ROUND(firstPairVolume, 2) AS firstPairVolume,
        secondPairExchange,
        secondPairMarket,
        baseAsset,
        quoteAsset,
        ROUND(secondPairPrice, 8) AS secondPairPrice,
        ROUND(secondPairVolume, 2) AS secondPairVolume,
        ROUND(secondPairPrice - firstPairPrice, 8) AS difference,
        -- Обмеження differencePercentage
        CASE 
            WHEN TRUNC(((secondPairPrice - firstPairPrice) / firstPairPrice) * 100, 2) > 1000000000 THEN 1000000000
            WHEN TRUNC(((secondPairPrice - firstPairPrice) / firstPairPrice) * 100, 2) < -1000000000 THEN -1000000000
            ELSE TRUNC(((secondPairPrice - firstPairPrice) / firstPairPrice) * 100, 2)
        END AS differencePercentage,
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
        d.firstPairPrice,
        d.firstPairVolume,
        d.secondPairExchange,
        d.secondPairMarket,
        d.secondPairPrice,
        d.secondPairVolume,
        d.difference,
        d.differencePercentage,
        CASE 
            WHEN d.differencePercentage > 0 
                THEN COALESCE(prev.timeOfLife, NOW() AT TIME ZONE 'UTC')
            ELSE NULL
        END AS timeOfLife,
        CASE 
            WHEN d.differencePercentage > 0 AND prev.timeOfLife IS NOT NULL
                THEN COALESCE(prev.timeElapsed, INTERVAL '0 seconds') + (NOW() AT TIME ZONE 'UTC' - prev.timeOfLife)
            ELSE INTERVAL '0 seconds'
        END AS timeElapsed,
        -- JSON для firstExchangeNetworks
        jsonb_build_object(
            'baseAsset', COALESCE((
                SELECT jsonb_agg(
                    jsonb_build_object(
                        'coin', n.coin,
                        'exchange', n.exchange,
                        'network', n.network,
                        'networkName', n.networkName,
                        'depositEnable', n.depositEnable,
                        'withdrawEnable', n.withdrawEnable,
                        'updatedAt', n.updatedAt
                    )
                ) 
                FROM nets n 
                WHERE n.exchange = d.firstPairExchange AND n.coin = d.baseAsset
            ), '[]'::jsonb),
            
            'quoteAsset', COALESCE((
                SELECT jsonb_agg(
                    jsonb_build_object(
                        'coin', n.coin,
                        'exchange', n.exchange,
                        'network', n.network,
                        'networkName', n.networkName,
                        'depositEnable', n.depositEnable,
                        'withdrawEnable', n.withdrawEnable,
                        'updatedAt', n.updatedAt
                    )
                ) 
                FROM nets n 
                WHERE n.exchange = d.firstPairExchange AND n.coin = d.quoteAsset
            ), '[]'::jsonb)
        ) AS firstExchangeNetworks,

        -- JSON для secondExchangeNetworks
        jsonb_build_object(
            'baseAsset', COALESCE((
                SELECT jsonb_agg(
                    jsonb_build_object(
                        'coin', n.coin,
                        'exchange', n.exchange,
                        'network', n.network,
                        'networkName', n.networkName,
                        'depositEnable', n.depositEnable,
                        'withdrawEnable', n.withdrawEnable,
                        'updatedAt', n.updatedAt
                    )
                ) 
                FROM nets n 
                WHERE n.exchange = d.secondPairExchange AND n.coin = d.baseAsset
            ), '[]'::jsonb),
            
            'quoteAsset', COALESCE((
                SELECT jsonb_agg(
                    jsonb_build_object(
                        'coin', n.coin,
                        'exchange', n.exchange,
                        'network', n.network,
                        'networkName', n.networkName,
                        'depositEnable', n.depositEnable,
                        'withdrawEnable', n.withdrawEnable,
                        'updatedAt', n.updatedAt
                    )
                ) 
                FROM nets n 
                WHERE n.exchange = d.secondPairExchange AND n.coin = d.quoteAsset
            ), '[]'::jsonb)
        ) AS secondExchangeNetworks,
        
        NOW() AT TIME ZONE 'UTC' AS updatedAt
    FROM calculated_diffs d
    LEFT JOIN diffs prev ON d.pairKey = prev.pairKey
)
INSERT INTO diffs (pairKey, symbol, baseAsset, quoteAsset, firstPairExchange, firstPairMarket, firstPairPrice, firstPairVolume, 
                   secondPairExchange, secondPairMarket, secondPairPrice, secondPairVolume, difference, 
                   differencePercentage, timeOfLife, timeElapsed, firstExchangeNetworks, secondExchangeNetworks, updatedAt)
SELECT 
    pairKey, symbol, baseAsset, quoteAsset, firstPairExchange, firstPairMarket, firstPairPrice, firstPairVolume, 
    secondPairExchange, secondPairMarket, secondPairPrice, secondPairVolume, difference, 
    differencePercentage, timeOfLife, timeElapsed, firstExchangeNetworks, secondExchangeNetworks, updatedAt
FROM updating_data
ON CONFLICT (pairKey) DO UPDATE 
SET 
    firstPairPrice = EXCLUDED.firstPairPrice,
    firstPairVolume = EXCLUDED.firstPairVolume,
    secondPairPrice = EXCLUDED.secondPairPrice,
    secondPairVolume = EXCLUDED.secondPairVolume,
    difference = EXCLUDED.difference,
    differencePercentage = EXCLUDED.differencePercentage,
    timeOfLife = EXCLUDED.timeOfLife,
    timeElapsed = EXCLUDED.timeElapsed,
    firstExchangeNetworks = EXCLUDED.firstExchangeNetworks,
    secondExchangeNetworks = EXCLUDED.secondExchangeNetworks,
    updatedAt = NOW() AT TIME ZONE 'UTC';
