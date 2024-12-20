Something like this:

```bash
adduser --system --group --home /opt/aqua --gecos 'Aquachain' aqua
install aquaproxy /opt/aqua/bin/aquaproxy
install docs/aquaproxy.service /etc/systemd/system/aquaproxy.service
install aquaproxy.example.conf /opt/aqua/aquaproxy.conf
systemctl enable aquaproxy
systemctl start aquaproxy
```
