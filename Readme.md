# PTouch-Web
A web interface to your PTouch printer

![Screenshot Multiline](https://user-images.githubusercontent.com/10907336/224796519-85a35ce7-be0c-41db-9288-47eabf4d665d.png)

- PTouch-Web uses the system fonts to render the shown image.
- The webinterface works without Javascript, however the font selection uses
JavaScript to change the `font=` parameter in the URL.
- It provides a `/print` `GET` endpoint for cli usage:
    ```bash
    curl http://labelprinter.local/print?fontsize=48&font=Ubuntu-L.ttf&label=Hello%20World
    ```
- The Font field is fuzzy searchable by font name and font family
### Connecting via USB
```
go build .
./ptouch-web usb
```
Ensure your user has the proper permissions to access /dev/bus/usb or change the permissions there

### Recommended fonts (debian):
- ttf-ubuntu-font-family
- ttf-mscorefonts-installer
- fonts-croscore
- fonts-liberation2
- fonts-freefont-ttf

```
sudo apt install ttf-ubuntu-font-family ttf-mscorefonts-installer fonts-freefont-ttf fonts-liberation2 fonts-croscore
```

### Deployment

You can create a overwrite `docker-compose.prod.yml` to map custom fonts or device:
```docker-compose
services:
  ptouch-web:
    volumes:
      - /usr/share/fonts/:/usr/local/share/fonts/:ro
    ports:
      - 80:8080
```
and run with
`docker-compose -f docker-compose.yml -f docker-compose.prod.yml up --build`

### Connecting an rfcomm node via bluetooth

After connecting to the device it will automatically disconnect again.
On linux you can list the device by using bluetooth-ctl's paired-devices listing.
e.g.
```
[bluetooth]# paired-devices
Device EC:79:49:65:XX:XX PT-P300BTXXXX
```

You can then create an rfcom device from the bl address:
```
sudo rfcomm bind 0 EC:79:49:65:XX:XX 1
```

Afterwards PTouch-Web can be passed the rfcomm device
```
ptouch-web /dev/rfcomm0
```
