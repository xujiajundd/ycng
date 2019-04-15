#include "libRaptorQ/cRaptorQ.h"
#include <stdio.h>

int test() {
    uint8_t *myvec;
    uint32_t mysize = 0;

    struct pair {
        uint32_t id;
        uint32_t *symbol;
    };

    // will keep "sent" symbols here.
    struct pair *encoded;

    // symbol and sub-symbol sizes
    const uint16_t subsymbol = 1024;
    const uint16_t symbol_size = 1024;

    /*
     * Now start decoding things
     */

    // use multiple blocks
    struct RaptorQ_ptr *enc = RaptorQ_Enc (ENC_8, myvec, mysize, subsymbol,
                                           symbol_size, 20000);

    if (enc == NULL) {
        fprintf(stderr, "Coud not initialize encoder.\n");
        //free (myvec);
        return NULL;
    }

    // start background precomputation while we get the source symbols.
    RaptorQ_precompute (enc, 1, false);

    /* everything is encoded now.
     * well, it might be running in background, but don't worry:
     * if needed you'll just block on the call.
     */

    float  drop_prob = 50.0;	// this is still too high probably.
    uint8_t overhead = 1;
    srand((uint32_t)time(NULL));

    // we used a lot of memory before so there will be only one block.
    // allocate memory only for that data.
    uint32_t symbols_tot = 0;
    for (uint8_t b = 0; b < RaptorQ_blocks (enc); ++b) {
        uint16_t sym = RaptorQ_symbols (enc, b);
        symbols_tot += (sym + overhead);
    }

    encoded = (struct pair *) malloc (sizeof(struct pair) * symbols_tot);
    for (uint32_t i = 0; i < symbols_tot; ++i)
        encoded[i].symbol = NULL;

    uint32_t next_encoded = 0;

    uint32_t blocks = RaptorQ_blocks (enc);
    for (uint8_t block = 0; block < blocks; ++block) {
        printf("Block: %i\n", block);
        uint32_t sym_for_blk = RaptorQ_symbols (enc, block);
        // some source packets will be lost. Be sure we will have
        // exactly (overhead + dropped_source) repair symbols.
        // and "sym_for_blk + overhead" total symbols
        int32_t repair = overhead;
        for (uint32_t source = 0; source < sym_for_blk; ++source) {
            float dropped = ((float)(rand()) / (float) 1000) * (float)100.0;
            if (dropped < drop_prob) {
                // dropped source symbol. Don't even get it.
                ++repair;
                continue;
            }
            // successfully transmitted source symbol. Add it to "encoded";
            encoded[next_encoded].id = RaptorQ_id (source, block);
            uint32_t data_size = symbol_size / sizeof(uint8_t);
            encoded[next_encoded].symbol = (uint32_t *) malloc (symbol_size);
            uint32_t *data = encoded[next_encoded].symbol;
            uint64_t written = RaptorQ_encode (enc, (void **)&data, data_size,
                                               source, (uint8_t)block);
            // "data" now points to "encoded[next_encoded].symbol + written"
            // you can use this for some tricks if you *really* need it.
            if (written != data_size) {
                fprintf(stderr, "Error in getting source symbol\n");
                //free (myvec);
                for (uint32_t k = 0; k <= next_encoded; ++k)
                    free (encoded[k].symbol);
                free (encoded);
                RaptorQ_free (&enc);
                return NULL;
            }
            ++next_encoded;
        }
        printf("Dropped %i source packets\n", repair - overhead);
        // some repair packets will be lost. Be sure we will have
        // exactly (overhead + dropped_source) repair symbols,
        // and "sym_for_blk + overhead" total symbols
        uint32_t sym_rep;
        for (sym_rep = sym_for_blk; repair > 0 &&
             sym_rep < RaptorQ_max_repair (enc, block); ++sym_rep) {
            // repair symbols can be dropped, too!
            float dropped = ((float)(rand()) / (float) 1000) * (float)100.0;
            if (dropped < drop_prob) {
                // dropped repair symbol. Don't even get it.
                continue;
            }
            --repair;
            // successfully transmitted repair symbol. Add it to "encoded";
            encoded[next_encoded].id = RaptorQ_id (sym_rep, block);
            uint32_t data_size = symbol_size / sizeof(uint8_t);
            encoded[next_encoded].symbol = (uint32_t *) malloc (symbol_size);
            uint32_t *data = encoded[next_encoded].symbol;
            uint64_t written = RaptorQ_encode (enc, (void **)&data, data_size,
                                               sym_rep, (uint8_t)block);
            // "*data" now points to "encoded[next_encoded].symbol + written"
            // you can use this for some tricks if you *really* need it.
            if (written != data_size) {
                fprintf(stderr, "Error in getting repair symbol\n");
                //free (myvec);
                for (uint32_t k = 0; k <= next_encoded; ++k)
                    free (encoded[k].symbol);
                free (encoded);
                RaptorQ_free (&enc);
                return NULL;
            }
            ++next_encoded;
        }
        if (sym_rep == RaptorQ_max_repair (enc, block)) {
            fprintf(stderr, "Maybe losing %f%% symbols is too much?\n",
                    drop_prob);
            //free (myvec);
            for (uint32_t k = 0; k < next_encoded; ++k)
                free (encoded[k].symbol);
            free (encoded);
            RaptorQ_free (&enc);
            return NULL;
        }
    }

    uint32_t oti_scheme = RaptorQ_OTI_Scheme (enc);
    uint64_t oti_common = RaptorQ_OTI_Common (enc);

    // optionally: free the memory just of block 0
    // this is done by the RaptorQ_free call anyway
    // RaptorQ_free_block (&enc, 0);
    // free the whole encoder memory:
    RaptorQ_free (&enc);
    // enc == NULL now


    /*
     * Now start decoding things
     */

    struct RaptorQ_ptr *dec = RaptorQ_Dec (DEC_8, oti_common, oti_scheme);

    if (dec == NULL) {
        fprintf(stderr, "Could not initialize decoder!\n");
        //free (myvec);
        for (uint32_t k = 0; k < next_encoded; ++k)
            free (encoded[k].symbol);
        free (encoded);
        return NULL;
    }

    // we made things so there is only one block.
    // if you want to generalize things,
    // RaptorQ_blocks (dec); will tell you how many blocks
    // there are.
    for (size_t i = 0; i < next_encoded; ++i) {
        uint32_t *data = encoded[i].symbol;
        uint32_t data_size = RaptorQ_symbol_size (dec) / sizeof(uint8_t);
        if (!RaptorQ_add_symbol_id (dec, (void **)&data, data_size,
                                    encoded[i].id)) {
            // this can happen if we receive the same symbol twice
            // (which doesn't make much sense). But we constructed
            // everything so that there are no duplicates here,
            // so this is some other error.
            fprintf(stderr, "Error: couldn't add the symbol to the decoder\n");
            //free (myvec);
            for (uint32_t k = 0; k < next_encoded; ++k)
                free (encoded[k].symbol);
            free (encoded);
            RaptorQ_free (&dec);
            return NULL;
        }
        // "data" now points to encoded[i].symbol +
        //					ceil(RaptorQ_symbol_size(dec) / sizeof(uint8_t))
    }

    // make sure that there's enough place in "received" to get the
    // whole decoded data.
    uint8_t *received = (uint8_t *) malloc (RaptorQ_bytes (dec));

    uint8_t *rec = received;
    // you can actually call "RaptorQ_decode" as many times as you want
    // until you get enough data. it will wait until it has enough data
    // to start the decoding.
    uint64_t written = RaptorQ_decode (dec, (void **)&rec,
                                       RaptorQ_bytes (dec) / sizeof(uint8_t));
    // "rec" now points to "received + written
    // This might help you to call RaptorQ_decode_sbn multiple time
    // on the same pointer.

    if (written != RaptorQ_bytes (dec) / sizeof(uint8_t)) {
        fprintf(stderr, "Couldn't decode: %i - %lu\n", mysize, written);
//        free (myvec);
        free(received);
        for (uint32_t k = 0; k < next_encoded; ++k)
            free (encoded[k].symbol);
        free (encoded);
        RaptorQ_free(&dec);
        return NULL;
    } else {
        printf("Decoded: %i\n", mysize);
    }
    // check if everything was decoded nicely
    for (uint32_t i = 0; i < mysize; ++i) {
        if (myvec[i] != received[i]) {
            fprintf(stderr, "FAILED, but we though otherwise! %i - %f: %i "
                    "%i -- %i\n",
                    mysize, drop_prob, i,
                    myvec[i], received[i]);
            //free (myvec);
            free (received);
            for (uint32_t k = 0; k < next_encoded; ++k)
                free (encoded[k].symbol);
            free (encoded);
            RaptorQ_free (&dec);
            return NULL;
        }
    }

    // optionally: free the memory just of block 0
    // this is done by the RaptorQ_free call anyway
    // RaptorQ_free_block (&dec, 0);
    // free the decoder memory
//    free (myvec);
//    free(received);
    for (uint32_t k = 0; k < next_encoded; ++k)
        free (encoded[k].symbol);
    free (encoded);
    RaptorQ_free(&dec);
}