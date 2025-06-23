# Setup for Scarf

This document outlines our implementation details for [Scarf](https://scarf.sh) which provides usage and download analytics for the Jaeger project. 

## DNS Configuration
The following CNAMES were setup in Netlify for us to utilize the services:

  * `scarf.jaegertracing.io` -> `static.scarf.sh` (used for tracking pixel on webpages)
  * `cr.jaegertracing.io` -> `gateway.scarf.sh` (used for container registries)
  * `download.jaegertracing.io` -> `gateway.scarf.sh` (used for file downloads of Jaeger artifacts)

We also had to add the following TXT verification records:
1. _scarf-sh-challenge-jaeger.cr.jaegertracing.io - ZN4RLVE3CENVUIXDBNYCa
2. _scarf-sh-challenge-jaeger.download.jaegertracing.io - U2GBROI64YGH2JLRTXPI
3. _scarf-sh-challenge-jaeger.scarf.jaegertracing.io - AKB26262A53WP55R4EXR

## Download and Docker Configuration
The following setup has been done on Scarf. Previously what was the download link for example https://github.com/jaegertracing/jaeger/releases/download/v1.69.0/jaeger-2.6.0-darwin-amd64.tar.gz should now be https://download.jaegertracing.io/v1.69.0/jaeger-2.6.0-darwin-amd64.tar.gz for us to get analytics.

For Docker containers the previous command : docker pull jaegertracing/all-in-one should now be docker pull cr.jaegertracing.io/jaegertracing/all-in-one

# Integrating Scarf Analytics with `www.jaegertracing.io` on Netlify

Injecting the Scarf analytics tracking pixel to the `www.jaegertracing.io` website hosted on Netlify is done using Netlify's **Snippet Injection** feature.

## Steps to Add the Scarf Pixel

1.  **Log in to Netlify:**

      * Head over to [app.netlify.com](https://app.netlify.com/) and sign in to your account which has access to the `www.jaegertracing.io` site configuration.

2.  **Select Your Site:**

      * From the list of sites, click on `www.jaegertracing.io`.

3.  **Go to Project Configuration:**

      * On the `www.jaegertracing.io` site dashboard, click on **"Site settings"** (you'll usually find this at the top right).

4.  **Access Build & Deploy Settings:**

      * In the left-hand sidebar, under "Site settings," click on **"Build & deploy."**

5.  **Find Snippet Injection:**

      * Scroll down to the "Post processing" section and find the **"Snippet Injection"** section.
      * Click the **"Add Snippet"** button.

6.  **Configure the Scarf Snippet:**

      * A form will pop up for your new snippet:
          * **Snippet Name:** Give it a clear name like `Scarf Tracking Pixel`.

          * **Position:** Choose **"Before `</body>`"**. This is generally a good spot for image-based pixels as it doesn't block the initial page render.

          * **Snippet Body:** Paste the following HTML code block into this text area. **The pixel ID `cf7517a5-bfa0-4796-b760-1bb4e302e541` is already included.**

            ```html
            <img referrerpolicy="no-referrer-when-downgrade" src="https://scarf.jaegertracing.io/a.png?x-pxid=cf7517a5-bfa0-4796-b760-1bb4e302e541" alt="" style="position: absolute; width: 0; height: 0; border: 0;" />
            ```

7.  **Save the Snippet:**

      * Click the **"Save"** button to apply the changes.

-----

## Verification

Once you save, Netlify will automatically inject this image tag into the HTML pages of `www.jaegertracing.io`. You won't need to trigger a new deployment. Scarf.sh should start receiving analytics data from the website shortly after.

To verify the pixel is loading correctly:

  * Visit `www.jaegertracing.io` in a browser.
  * Open your browser's **developer tools** (usually by pressing F12 or right-clicking and selecting "Inspect").
  * Go to the **"Network"** tab and filter by "a.png". You should see requests being made to `https://scarf.jaegertracing.io/a.png`.
  