# GHS pictograms

The nine hazard pictograms from the UN Globally Harmonized System, as adopted by
OSHA in 29 CFR 1910.1200 Appendix C.

Converted from the official EPS artwork (Adobe Illustrator source, titles such as
"EXPLODING BOMB" and "CORROSION" preserved in the original files) via:

    gs -dEPSCrop -sDEVICE=pdfwrite   # EPS -> PDF
    pdf2svg <in>.pdf <out>.svg 1     # PDF -> SVG

Each file is true vector: paths only, no embedded rasters.

Filenames are the pictogram code, and the code-to-symbol mapping was verified
three ways before use -- filename, the title embedded in the source EPS, and
visual inspection of the rendered artwork. A pictogram attached to the wrong
hazard class on a safety data sheet is a serious error, so re-verify visually if
these files are ever replaced.
