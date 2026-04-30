#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int tinygo_org_pio_38(const char *input, char *output, size_t out_size) {
    if (input == NULL || output == NULL || out_size == 0) { return -1; }
    strncpy(output, input, out_size - 1);
    output[out_size - 1] = '\0';
    return 0;
}

int main(void) {
    char buf[256];
    if (tinygo_org_pio_38("ok", buf, sizeof(buf)) == 0) {
        printf("%s\n", buf);
    }
    return 0;
}
