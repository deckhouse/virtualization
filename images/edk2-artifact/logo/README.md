## Logo

ImageMagick command to convert from png:

```
convert Logo.png -alpha off -compress none BMP:Logo.bmp
```

Logo should be uncompressed BMP file. 8bit or 24 bit. 400x250px is great.
Use bmp-info script to check BMP file. Compression should be 0:

```
./bmp-info.py Logo.bmp
Width: 400
Height: 250
BitCount: 24
Compression: 0
```

This will not work and build will fail:
```
./bmp-info.py Logo.bmp
Width: 400
Height: 250
BitCount: 32
Compression: 3
```
