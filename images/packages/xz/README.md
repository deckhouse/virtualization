# xz
/xz
```
`-- usr
    |-- bin
    |   |-- lzcat -> xz
    |   |-- lzma -> xz
    |   |-- lzmadec
    |   |-- lzmainfo
    |   |-- unlzma -> xz
    |   |-- unxz -> xz
    |   |-- xz
    |   |-- xzcat -> xz
    |   `-- xzdec
    |-- include
    |   |-- lzma
    |   |   |-- base.h
    |   |   |-- bcj.h
    |   |   |-- block.h
    |   |   |-- check.h
    |   |   |-- container.h
    |   |   |-- delta.h
    |   |   |-- filter.h
    |   |   |-- hardware.h
    |   |   |-- index.h
    |   |   |-- index_hash.h
    |   |   |-- lzma12.h
    |   |   |-- stream_flags.h
    |   |   |-- version.h
    |   |   `-- vli.h
    |   `-- lzma.h
    |-- lib64
    |   |-- liblzma.a
    |   |-- liblzma.la
    |   |-- liblzma.so -> liblzma.so.5.4.5
    |   |-- liblzma.so.5 -> liblzma.so.5.4.5
    |   |-- liblzma.so.5.4.5
    |   `-- pkgconfig
    |       `-- liblzma.pc
    `-- share
        |-- doc
        |   `-- xz
        |       |-- AUTHORS
        |       |-- COPYING
        |       |-- COPYING.GPLv2
        |       |-- NEWS
        |       |-- README
        |       |-- THANKS
        |       |-- TODO
        |       |-- api
        |       |   |-- annotated.html
        |       |   |-- base_8h.html
        |       |   |-- bc_s.png
        |       |   |-- bc_sd.png
        |       |   |-- bcj_8h.html
        |       |   |-- block_8h.html
        |       |   |-- check_8h.html
        |       |   |-- classes.html
        |       |   |-- closed.png
        |       |   |-- container_8h.html
        |       |   |-- delta_8h.html
        |       |   |-- dir_b17a1d403082bd69a703ed987cf158fb.html
        |       |   |-- doc.svg
        |       |   |-- docd.svg
        |       |   |-- doxygen.css
        |       |   |-- doxygen.svg
        |       |   |-- doxygen_crawl.html
        |       |   |-- files.html
        |       |   |-- filter_8h.html
        |       |   |-- folderclosed.svg
        |       |   |-- folderclosedd.svg
        |       |   |-- folderopen.svg
        |       |   |-- folderopend.svg
        |       |   |-- functions.html
        |       |   |-- functions_vars.html
        |       |   |-- globals.html
        |       |   |-- globals_defs.html
        |       |   |-- globals_enum.html
        |       |   |-- globals_eval.html
        |       |   |-- globals_func.html
        |       |   |-- globals_type.html
        |       |   |-- hardware_8h.html
        |       |   |-- index.html
        |       |   |-- index_8h.html
        |       |   |-- index__hash_8h.html
        |       |   |-- lzma12_8h.html
        |       |   |-- lzma_8h.html
        |       |   |-- minus.svg
        |       |   |-- minusd.svg
        |       |   |-- nav_f.png
        |       |   |-- nav_fd.png
        |       |   |-- nav_g.png
        |       |   |-- nav_h.png
        |       |   |-- nav_hd.png
        |       |   |-- navtree.css
        |       |   |-- open.png
        |       |   |-- plus.svg
        |       |   |-- plusd.svg
        |       |   |-- splitbar.png
        |       |   |-- splitbard.png
        |       |   |-- stream__flags_8h.html
        |       |   |-- structlzma__allocator.html
        |       |   |-- structlzma__block.html
        |       |   |-- structlzma__filter.html
        |       |   |-- structlzma__index__iter.html
        |       |   |-- structlzma__mt.html
        |       |   |-- structlzma__options__bcj.html
        |       |   |-- structlzma__options__delta.html
        |       |   |-- structlzma__options__lzma.html
        |       |   |-- structlzma__stream.html
        |       |   |-- structlzma__stream__flags.html
        |       |   |-- sync_off.png
        |       |   |-- sync_on.png
        |       |   |-- tab_a.png
        |       |   |-- tab_ad.png
        |       |   |-- tab_b.png
        |       |   |-- tab_bd.png
        |       |   |-- tab_h.png
        |       |   |-- tab_hd.png
        |       |   |-- tab_s.png
        |       |   |-- tab_sd.png
        |       |   |-- tabs.css
        |       |   |-- version_8h.html
        |       |   `-- vli_8h.html
        |       |-- examples
        |       |   |-- 00_README.txt
        |       |   |-- 01_compress_easy.c
        |       |   |-- 02_decompress.c
        |       |   |-- 03_compress_custom.c
        |       |   |-- 04_compress_easy_mt.c
        |       |   `-- Makefile
        |       |-- examples_old
        |       |   |-- xz_pipe_comp.c
        |       |   `-- xz_pipe_decomp.c
        |       |-- faq.txt
        |       |-- history.txt
        |       |-- lzma-file-format.txt
        |       `-- xz-file-format.txt
        |-- locale
        |   |-- ca
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- cs
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- da
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- de
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- eo
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- es
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- fi
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- fr
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- hr
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- hu
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- it
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- ko
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- pl
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- pt
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- pt_BR
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- ro
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- sr
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- sv
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- tr
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- uk
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- vi
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   |-- zh_CN
        |   |   `-- LC_MESSAGES
        |   |       `-- xz.mo
        |   `-- zh_TW
        |       `-- LC_MESSAGES
        |           `-- xz.mo
        `-- man
            |-- de
            |   `-- man1
            |       |-- lzcat.1 -> xz.1
            |       |-- lzma.1 -> xz.1
            |       |-- lzmadec.1 -> xzdec.1
            |       |-- unlzma.1 -> xz.1
            |       |-- unxz.1 -> xz.1
            |       |-- xz.1
            |       |-- xzcat.1 -> xz.1
            |       `-- xzdec.1
            |-- fr
            |   `-- man1
            |       |-- lzcat.1 -> xz.1
            |       |-- lzma.1 -> xz.1
            |       |-- lzmadec.1 -> xzdec.1
            |       |-- unlzma.1 -> xz.1
            |       |-- unxz.1 -> xz.1
            |       |-- xz.1
            |       |-- xzcat.1 -> xz.1
            |       `-- xzdec.1
            |-- ko
            |   `-- man1
            |       |-- lzcat.1 -> xz.1
            |       |-- lzma.1 -> xz.1
            |       |-- lzmadec.1 -> xzdec.1
            |       |-- unlzma.1 -> xz.1
            |       |-- unxz.1 -> xz.1
            |       |-- xz.1
            |       |-- xzcat.1 -> xz.1
            |       `-- xzdec.1
            |-- man1
            |   |-- lzcat.1 -> xz.1
            |   |-- lzma.1 -> xz.1
            |   |-- lzmadec.1 -> xzdec.1
            |   |-- lzmainfo.1
            |   |-- unlzma.1 -> xz.1
            |   |-- unxz.1 -> xz.1
            |   |-- xz.1
            |   |-- xzcat.1 -> xz.1
            |   `-- xzdec.1
            |-- pt_BR
            |   `-- man1
            |       |-- lzcat.1 -> xz.1
            |       |-- lzma.1 -> xz.1
            |       |-- lzmadec.1 -> xzdec.1
            |       |-- unlzma.1 -> xz.1
            |       |-- unxz.1 -> xz.1
            |       |-- xz.1
            |       |-- xzcat.1 -> xz.1
            |       `-- xzdec.1
            |-- ro
            |   `-- man1
            |       |-- lzcat.1 -> xz.1
            |       |-- lzma.1 -> xz.1
            |       |-- lzmadec.1 -> xzdec.1
            |       |-- unlzma.1 -> xz.1
            |       |-- unxz.1 -> xz.1
            |       |-- xz.1
            |       |-- xzcat.1 -> xz.1
            |       `-- xzdec.1
            `-- uk
                `-- man1
                    |-- lzcat.1 -> xz.1
                    |-- lzma.1 -> xz.1
                    |-- lzmadec.1 -> xzdec.1
                    |-- unlzma.1 -> xz.1
                    |-- unxz.1 -> xz.1
                    |-- xz.1
                    |-- xzcat.1 -> xz.1
                    `-- xzdec.1

74 directories, 203 files
```