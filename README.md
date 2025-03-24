# UMP Parser

Parse YouTube's undocumented `UMP` media response format.  
Supports Part 20 (`MEDIA_HEADER`) and Part 21 (`MEDIA`) parsing.

The `MEDIA` section is extracted as a raw byte slice. Save it as a `.webm` file — it’ll contain the audio stream.  
Both single-chunk and multi-chunk UMP responses are supported.

> ⚠️ Note: UMP responses are sometimes split across multiple parts and may not contain the full media.  
> If you provide all response chunks, the decoder will return the full, reconstructed data.
> Chunk 0 is always required to get any media data. 

![VoidObscura UMP Parser](./umpparser.png)

---

## 🔍 Overview

This tool decodes the binary protobuf payloads found in YouTube's UMP responses.  
It currently supports:

- ✅ `MEDIA_HEADER` decoding (Part 20)
- ✅ `MEDIA` data extraction (Part 21)
- ✅ Reassembly of partial UMP responses