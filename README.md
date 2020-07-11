# Image resizer

**Created as part of [inspishop][link-inspishop] e-commerce platform by [inspirum][link-inspirum] team.**

[![Software License][ico-license]][link-licence]

On-demand image resizing, format converting and size optimization with the best CLI tools. 
Prepared to be used as Docker image with cloud storage as Amazon S3.

- image [resizing](#resizing), [format converting](#format-encoding) and [optimization](#optimization)
- support for **jpeg**, **png**, **webp**, **bmp**, **gif**, **tiff**, **svg** image formats
- file-cache for resized images on local disk *(temporary cache)* and cloud storage *(persistent cache)*


## Usage example


### Resizing

Application handle route with dynamic [**template**](#template) (parameters divided by dash) and **filepath** parameters.
```
GET /image/{template}/{filepath}
```


#### Template

Options for resizing are specified as a dash delimited list of parameters, which can be supplied in any order. 
Duplicate parameters overwrite last values.

| Name | Template | Example | Description  |
| :--- | :--- | :--- | :--- |
| width | `w[X]` | `w200` | Resize to 200px width with proportional height |
| height| `h[X]` | `h100` | Resize to 100px height with proportional width |
| ratio | `[X]x[X]` | `1x2` |  Resize to ratio to 1:2 *(width : height)* |
| crop | `crop` | `crop` | Image fill whole canvas (no addition background added) |
| upscale | `upscale` | `upscale` | Image can be larger than original |

There are special template string `original`. This will return optimized image with original dimensions.
It is helpful to show image with maximal resolution without knowing its dimension.

When there are set both `height` and `width` then `ratio` parameter will be ignored.

If output image has a different ratio than an original, image will be centered on transparent canvas (**jpg** has white background because it doesn't support transparent color).


#### Examples

> Values are for an image with dimension 200x400px (width : height).

| Template string | Output dimension |
| :--- | :--- | 
| `original` | 200x400 |
| `h100` | 50x100 |
| `w100` |  100x200 |
| `w50-w200` |  100x200 |
| `w100-h300` | 100x300 |
| `w200-1x1` | 200x200 |
| `w200-1x3` | 200x600 |
| `w100-h300-2x1` | 100x300 |
| `w1000-1x1` | 1000x1000 |
| `w1000-1x1-crop` | 200x200 |
| `w1000-1x1-crop-upscale` | 1000x1000 |


#### Width & height

> Examples use **jpg** format because canvas background is white (for better visibility)

| `h200` | `w200` | `w188-h233` |
| :---: | :---: | :---: |
| *(300x200)* | *(200x134)*  | *(188x233)* |
| ![h200](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/sizes/h200.jpg) | ![w200](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/sizes/w200.jpg) | ![w188-h233](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/sizes/w188-h233.jpg) |


#### Ratio


| `h200-1x1` | `h200-1x2` | `h200-2x1` |
| :---: | :---: | :---: |
| *(200x200)* | *(100x200)*  | *(400x200)* |
| ![h200-1x1](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/ratio/h200-1x1.jpg) | ![h200-1x2](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/ratio/h200-1x2.jpg) | ![h200-121](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/ratio/h200-2x1.jpg) |


#### Crop

> `crop` parameter will crop image to fill whole canvas 

| `h200-1x1-crop` | `h200-1x2-crop` | `h200-2x1-crop` |
| :---: | :---: | :---: |
| *(200x200)* | *(100x200)*  | *(400x200)* |
| ![h200-1x1-crop](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/ratio/h200-1x1-crop.jpg) | ![h200-1x2-crop](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/ratio/h200-1x2-crop.jpg) | ![h200-121-crop](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/ratio/h200-2x1-crop.jpg) |


#### Upscale

> By default, image can't up-size (be larger than original) 

| `original` | `w250-h300` | `w250-h300-upscale` | `w250-h300-crop` | `w250-h300-crop-upscale` |
| :---: | :---: | :---: | :---: | :---: |
| *(200x200)* | *(250x300)*  | *(250x300)* | *(167x200)* | *(250x300)*  |
| ![original](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/original.jpg) | ![w250-h300](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/w250-h300.jpg) | ![w250-h300-upscale](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/w250-h300-upscale.jpg) | ![w250-h300-crop](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/w250-h300-crop.jpg) | ![w250-h300-crop-upscale](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/w250-h300-crop-upscale.jpg) |

| `original` | `w500-h300-crop` |`w500-h300-crop` | `w500-h300-crop-upscale` | 
| :---: | :---: | :---: | :---: |
| *(200x200)* |  *(500x300)* | *(200x120)* | *(500x300)* |
| ![original](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/original.jpg) | ![w500-h300](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/w500-h300.jpg) | ![w500-h300-crop](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/w500-h300-crop.jpg) | ![w5w500-h300-crop-upscale](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/upsize/w500-h300-crop-upscale.jpg) |


### Format encoding

Image can be encoded to any supported format simple by changing file extension in URL.

Original image extension is specified in `original` query parameter.

> This is typically used to encode image as web-friendly **webp** format
```
http://localhost:3000/image/custom-w400-1x1-upscale/data/products/0/1/background.webp?original=png
```


### Optimization

The package will use these optimizers if they are present on your system:

- [pngquant 2](https://pngquant.org)
- [jpegoptim](http://freecode.com/projects/jpegoptim)
- [gifsicle](http://www.lcdf.org/gifsicle)
- [svgcleaner](https://github.com/RazrFalcon/svgcleaner)
- [cwebp](https://developers.google.com/speed/webp/docs/cwebp)

By setting environment variable you can change default path to binary file or default arguments. 

| Environment variable | Default value |
| :--- | :--- | 
| `CMD_OPTIMIZER_PNG` | `pngquant --force --ext .png --skip-if-larger --quality 0-75 --speed 4 --strip --` |
| `CMD_OPTIMIZER_JPG` | `jpegoptim --force --strip-all --max 75 --quiet --all-progressive` |
| `CMD_OPTIMIZER_GIF` | `gifsicle --batch --optimize=3` |
| `CMD_OPTIMIZER_SVG` | `svgcleaner` |
| `CMD_OPTIMIZER_WEBP` | `cwebp -m 6 -pass 10 -mt -q 75 -quiet` |


#### Examples

| 64 kB | 28 kB *(43 %)* | 23 kB *(5 %)* |
| :---: | :---: | :---: |
| ![original](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/optimize/jpg_optimized.jpg) | ![optimized](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/optimize/jpg_optimized.jpg) | ![optimized](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/optimize/jpg_optimized.webp) |

| 421 kB | 91 kB *(21 %)* | 23 kB *(5 %)* |
| :---: | :---: | :---: |
| ![original](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/optimize/png_original.png) | ![optimized](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/optimize/png_optimized.png) | ![optimized](https://github.com/inspirum/assets/raw/master/image-resizer-go/img/optimize/png_optimized.webp) |


## System requirements


## Installation


## Testing


## Contributing

Please see [CONTRIBUTING][link-contributing] and [CODE_OF_CONDUCT][link-code-of-conduct] for details.


## Security

If you discover any security related issues, please email tomas.novotny@inspirum.cz instead of using the issue tracker.


## Credits

- [Tomáš Novotný](https://github.com/tomas-novotny)
- [All Contributors][link-contributors]


## License

The MIT License (MIT). Please see [License File][link-licence] for more information.


[ico-license]:              https://img.shields.io/github/license/inspirum/image-resizer-go.svg?style=flat-square&colorB=blue
[ico-travis]:               https://img.shields.io/travis/inspirum/image-resizer-go/master.svg?branch=master&style=flat-square

[link-author]:              https://github.com/inspirum
[link-contributors]:        https://github.com/inspirum/image-resizer-go/contributors
[link-licence]:             ./LICENSE.md
[link-changelog]:           ./CHANGELOG.md
[link-contributing]:        https://github.com/inspirum/assets/raw/master/image-resizer-go/img/CONTRIBUTING.md
[link-code-of-conduct]:     https://github.com/inspirum/assets/raw/master/image-resizer-go/img/CODE_OF_CONDUCT.md
[link-travis]:              https://travis-ci.org/inspirum/image-resizer-go
[link-inspishop]:           https://www.inspishop.cz/
[link-inspirum]:            https://www.inspirum.cz/
