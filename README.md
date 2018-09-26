# Open Source Metaverse Mining Pool Orchestrator

Orchestrates all the pool functionality:
* Stratum
* Web API
* Payouts
* Block Unlocker

## Prerequisites

These are supported, other versions are unsupported.

* [mvsd](https://github.com/mvs-org/metaverse) v3 API
* git - all versions supported
* Ubuntu - 16.04.5 LTS
* go 1.6.x
* redis-server 3.0.x

If you also want the web interface, you'll also need to make your own or use [open-metaverse-pool-www](https://github.com/NotoriousPyro/open-metaverse-pool-www)

## Installation

Use <code>git</code> to download this repo to a folder, then <code>cd</code> to it:

    git clone git@github.com:NotoriousPyro/open-metaverse-pool.git /Your/Destination/Folder
    cd /Your/Destination/Folder

Now use <code>make</code> to build the pool, it will be placed in <code>build/bin</code>.

Configure the <code>.json</code> files accordingly, setting the wallet username and password, ports and various other settings.

Now use the <code>.service</code> files in <code>misc</code> to add the services to systemd. **Make sure you set the paths in these files**

    cp misc/*.service /etc/systemd/system/
    systemctl enable oep-etp-api
    systemctl enable oep-etp-stratum
    systemctl enable oep-etp-unlocker
    systemctl enable oep-etp-payouts

## Running / Usage

Now just start the services you added using:

    systemctl start oep-etp-*

If you get errors, please check folder permissions, missing folders, wallet is running, ports are open, and other common problems PRIOR to raising an issue. Issues raised with no prior debugging will be closed.

To build the Orchestrator, use <code>make</code> after a fresh install or when you make a change.