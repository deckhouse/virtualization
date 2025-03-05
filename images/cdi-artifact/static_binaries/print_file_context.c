#include <stdio.h>
#include <stdlib.h>

int main(int argc, char *argv[]) {
    FILE *fptr;
    char myContent[100];
    // Check for correct command-line arguments
    if (argc != 2) {
        printf("Usage: %s <filename>\n", argv[0]);
        return 1;
    }

    fptr = fopen(argv[1], "r"); // Open in read mode

    if(fptr != NULL) {
        // Read the content and print it
        while (fgets(myContent,100,fptr)) {
            printf("%s", myContent);
        }
    } else {
        perror("Not able to open the file");
        fclose(fptr);
        return 1;
    }
      

    fclose(fptr); // Close the file
    return 0;
}