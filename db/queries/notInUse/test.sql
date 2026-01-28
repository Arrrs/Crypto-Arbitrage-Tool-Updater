-- SELECT * 
-- FROM pairs
-- WHERE symbol IN ('NILUSDT')
-- WHERE symbol IN ('BTCUSDT')
-- WHERE symbol LIKE 'BTC%';
-- WHERE symbol IN ('BTCUSDT')
-- WHERE exchange IN ('WhiteBIT')
	-- AND symbol LIKE '%BTC%';
	-- AND baseasset IN ('BTC')
	-- AND quoteasset IN ('USDC')
-- ;
-- DELETE FROM pairs 
-- WHERE exchange = 'OKX'
-- ;
-- SELECT DISTINCT exchange 
-- FROM pairs


-- SELECT exchange, COUNT(symbol) AS symbol_count
-- FROM pairs
-- GROUP BY exchange
-- ORDER BY symbol_count DESC;



-- SELECT DISTINCT exchange 
-- FROM nets


-- SELECT * FROM nets;
-- SELECT * FROM diffs;
	-- WHERE baseasset ='N/A'

-- ALTER TABLE diffs
-- ADD COLUMN baseAsset VARCHAR(20) NOT NULL DEFAULT 'N/A',
-- ADD COLUMN quoteAsset VARCHAR(20) NOT NULL DEFAULT 'N/A';

-- ALTER TABLE nets
-- ADD COLUMN updatedAt TIMESTAMP NOT NULL DEFAULT timezone('UTC', CURRENT_TIMESTAMP);

-- ALTER TABLE nets
-- ADD COLUMN updatedAt TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;

-- ALTER TABLE nets
-- ALTER COLUMN updatedAt SET DEFAULT timezone('UTC', CURRENT_TIMESTAMP);

-- UPDATE nets
-- SET updatedAt = timezone('UTC', updatedAt);

-- ALTER TABLE nets
-- DROP COLUMN updatedAt;

-- ALTER TABLE diffs 
-- ADD COLUMN firstExchangeNetworks JSONB DEFAULT '{}'::JSONB,
-- ADD COLUMN secondExchangeNetworks JSONB DEFAULT '{}'::JSONB;


DELETE FROM pairs;
DELETE FROM diffs;
DELETE FROM nets;