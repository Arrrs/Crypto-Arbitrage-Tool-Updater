UPDATE diffs d
SET firstExchangeNetworks = jsonb_build_object(
    'baseAsset', COALESCE((
        SELECT jsonb_agg(
            jsonb_build_object(
                'coin', n.coin,
                'exchange', n.exchange,
                'network', n.network,
                'networkName', n.networkName,
                'depositEnable', n.depositEnable,
                'withdrawEnable', n.withdrawEnable
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
                'withdrawEnable', n.withdrawEnable
            )
        ) 
        FROM nets n 
        WHERE n.exchange = d.firstPairExchange AND n.coin = d.quoteAsset
    ), '[]'::jsonb)
);

UPDATE diffs d
SET secondExchangeNetworks = jsonb_build_object(
    'baseAsset', COALESCE((
        SELECT jsonb_agg(
            jsonb_build_object(
                'coin', n.coin,
                'exchange', n.exchange,
                'network', n.network,
                'networkName', n.networkName,
                'depositEnable', n.depositEnable,
                'withdrawEnable', n.withdrawEnable
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
                'withdrawEnable', n.withdrawEnable
            )
        ) 
        FROM nets n 
        WHERE n.exchange = d.secondPairExchange AND n.coin = d.quoteAsset
    ), '[]'::jsonb)
);
