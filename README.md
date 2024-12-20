# aquachain-proxy

Aquachain mining proxy with web-interface.

**Proxy feature list:**

* Rigs availability monitoring
* Keep track of accepts, rejects, blocks stats
* Easy detection of sick rigs
* Daemon failover list

![Demo](https://raw.githubusercontent.com/aquachain/aquachain-proxy/master/docs/screenshot.png)

### Installation

  1. Compile (eg: 'make' or 'make all' for cross-compilation)
  2. Copy aquachain-proxy to /usr/local/bin/
  3. Copy aquaproxy.example.json to /etc/aquaproxy.json, edit the YOUR_ADDRESS_HERE field (see Configuration below)
  4. Copy docs/aquaproxy.service to /etc/systemd/system/
  5. Run 'systemctl enable aquaproxy' to start on boot
  6. Run 'systemctl start aquaproxy' to start now
  7. Optional: Customize a frontend just by creating a 'www' directory in the working directory. Copy it from the repository's 'www' directory.

### Configuration

Configuration is self-describing, just copy *config.example.json* to *config.json* and specify endpoint URL and upstream URLs.

Or, use '-mkcfg' flag to create the file with defaults. (still need to change upstream)

#### Example upstream section

```json
{
  "upstream": [
    {
      "pool": true,
      "name": "EuroHash.net",
      "url": "http://eth-eu.eurohash.net:8888/miner/0xb85150eb365e7df0941f0cf08235f987ba91506a/proxy",
      "timeout": "10s"
    },
    {
      "name": "backup-geth",
      "url": "http://127.0.0.1:8545",
      "timeout": "10s"
    }
  ]
}
```

In this example we specified a mining pool as main mining target and a local geth node as backup for solo.

#### Running

With no arguments, aquachain-proxy will look for a aquaproxy.json in the working directory, /etc/aquaproxy.json, and /opt/aqua/aquaproxy.json

    ./aquachain-proxy

Specify a config file with the -cfg flag

    ./aquachain-proxy -cfg /etc/aquaproxy.json

If you have a 'www' directory but still want to serve from embedded filesystem,

    ./aquachain-proxy -e -cfg /etc/aquaproxy.json

#### Mining software configuration

    aquaminer-gpu -F http://x.x.x.x:8546/rig1 
    aquaminer-gpu -F http://x.x.x.x:8546/0.1/rig2 
    aquaminer-gpu -F http://x.x.x.x:8546/0x...1234/rig3 

### Pools that work with this proxy

* [See Explorer](https://aquachain.github.io/explorer/#/pool) for a list of active pools. (to add a pool, submit PR to [aquachain.github.io](https://github.com/aquachain/aquachain.github.io/blob/master/public/pools.json) source)




## Below is from [fork origin's README.md](https://github.com/sammy007/ether-proxy):

### Donations

* **ETH**: [0xb85150eb365e7df0941f0cf08235f987ba91506a](https://etherchain.org/account/0xb85150eb365e7df0941f0cf08235f987ba91506a)

* **BTC**: [1PYqZATFuYAKS65dbzrGhkrvoN9au7WBj8](https://blockchain.info/address/1PYqZATFuYAKS65dbzrGhkrvoN9au7WBj8)

Thanks to a couple of dudes who donated some Ether to me, I believe, you can do the same.

### License

The MIT License (MIT).
